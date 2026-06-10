package tests

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"

	awsease "github.com/aura-studio/aws-ease"
)

// TestParseErrorsViaInvoke 通过 Invoke 的可观察错误覆盖地址解析的各类非法输入
// （解析在分发前完成，故无需任何后端客户端）。
func TestParseErrorsViaInvoke(t *testing.T) {
	c := awsease.New()
	ctx := context.Background()

	tests := []struct {
		name   string
		target string
		want   error
	}{
		{"empty", "", awsease.ErrBadTarget},
		{"no scheme", "order-create", awsease.ErrBadTarget},
		{"empty scheme", "://x", awsease.ErrBadTarget},
		{"http missing host", "http://", awsease.ErrBadTarget},
		{"lambda missing name", "lambda://", awsease.ErrBadTarget},
		{"lambda path without name", "lambda:///api/x", awsease.ErrBadTarget},
		{"lambda empty tunnel path", "lambda://fn/", awsease.ErrBadTarget},
		{"lambda empty path segment", "lambda://fn/api//x", awsease.ErrBadTarget},
		{"lambda trailing slash", "lambda://fn/api/x/", awsease.ErrBadTarget},
		{"lambda unknown feature key", "lambda://fn/api/x?envelope=service", awsease.ErrBadTarget},
		{"lambda bad async value", "lambda://fn?async=yes", awsease.ErrBadTarget},
		{"sqs missing queue", "sqs://", awsease.ErrBadTarget},
		{"unknown scheme", "ftp://host", awsease.ErrUnknownScheme},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := c.Invoke(ctx, tt.target, nil)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want errors.Is %v", err, tt.want)
			}
			if body != nil {
				t.Errorf("body must be nil on parse error, got %q", body)
			}
		})
	}
}

// TestLambdaQueryIsFeatureParams 验证 "?" 后整段是特性参数：不泄漏进 FunctionName，
// 且（v0.5.0 行为变化）只认已知键——特性参数现在决定线上格式，未知键如 ?x=1
// 必须显式失败（ErrBadTarget）而非静默忽略。
func TestLambdaQueryIsFeatureParams(t *testing.T) {
	fl := &fakeLambda{out: &awslambda.InvokeOutput{Payload: []byte("ok")}}
	c := awsease.New(awsease.WithLambdaAPI(fl))

	body, err := c.Invoke(context.Background(), "lambda://fn?async=false", []byte("p"))
	if err != nil {
		t.Fatalf("lambda://fn?async=false must be legal: %v", err)
	}
	if awssdk.ToString(fl.in.FunctionName) != "fn" {
		t.Errorf("FunctionName = %q, want fn (feature params must not leak into the name)",
			awssdk.ToString(fl.in.FunctionName))
	}
	if string(body) != "ok" {
		t.Errorf("body = %q, want ok", body)
	}

	if _, err := c.Invoke(context.Background(), "lambda://fn?x=1", []byte("p")); !errors.Is(err, awsease.ErrBadTarget) {
		t.Errorf("lambda://fn?x=1 err = %v, want ErrBadTarget (unknown feature key must fail fast)", err)
	}
}

// TestRoutesByScheme 验证 scheme 决定后端。Backend 已不再导出，
// 路由正确性改用「哪个 fake / 服务器被实际调用」来断言。
func TestRoutesByScheme(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("http-ok"))
	}))
	defer srv.Close()

	fl := &fakeLambda{out: &awslambda.InvokeOutput{Payload: []byte("lambda-ok")}}
	fs := &fakeSQS{sendOut: &awssqs.SendMessageOutput{MessageId: awssdk.String("m")}}
	c := awsease.New(awsease.WithLambdaAPI(fl), awsease.WithSQSAPI(fs))
	ctx := context.Background()

	// http://… 走 HTTP：返回体证明真实服务器被打到，两个 fake 都不被触碰。
	if body, err := c.Invoke(ctx, srv.URL+"/ping", nil); err != nil || string(body) != "http-ok" {
		t.Fatalf("http: body=%q err=%v", body, err)
	}
	if fl.in != nil || fs.sendIn != nil {
		t.Fatalf("http call leaked to aws fakes: lambda=%v sqs=%v", fl.in, fs.sendIn)
	}

	// lambda://… 走 fakeLambda。
	if body, err := c.Invoke(ctx, "lambda://order-create", []byte("x")); err != nil || string(body) != "lambda-ok" {
		t.Fatalf("lambda: body=%q err=%v", body, err)
	}
	if fl.in == nil {
		t.Fatal("lambda fake was not invoked")
	}

	// sqs://… 走 fakeSQS，推送语义成功返回 (nil, nil)。
	body, err := c.Invoke(ctx, "sqs://https://sqs.test/q", []byte("x"))
	if err != nil || body != nil {
		t.Fatalf("sqs: body=%q err=%v, want (nil, nil)", body, err)
	}
	if fs.sendIn == nil {
		t.Fatal("sqs fake was not invoked")
	}
}

// TestLocalRedirect 验证 WithLocalRedirect 下 lambda:// 实际改走 HTTP mock 的
// /lambda/<fn> 路径（服务器把 path 写进响应体来断言），且 lambda 客户端不被调用。
func TestLocalRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.URL.Path))
	}))
	defer srv.Close()

	fl := &fakeLambda{out: &awslambda.InvokeOutput{Payload: []byte("should-not-run")}}
	c := awsease.New(awsease.WithLocalRedirect(srv.URL), awsease.WithLambdaAPI(fl))

	body, err := c.Invoke(context.Background(), "lambda://order-create", []byte("x"))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if string(body) != "/lambda/order-create" {
		t.Errorf("server path = %q, want /lambda/order-create", body)
	}
	if fl.in != nil {
		t.Error("lambda client must not be called when redirected")
	}
}
