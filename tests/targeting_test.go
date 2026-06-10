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
		{"lambda with path", "lambda://fn/extra", awsease.ErrBadTarget},
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

// TestLambdaQueryIsFeatureParams 验证 lambda://fn?x=1 现在是【合法】地址（v0.4.0 行为变化）：
// "?" 后整段是特性参数，不再被视为非法函数名，也不会泄漏进 FunctionName。
func TestLambdaQueryIsFeatureParams(t *testing.T) {
	fl := &fakeLambda{out: &awslambda.InvokeOutput{Payload: []byte("ok")}}
	c := awsease.New(awsease.WithLambdaAPI(fl))

	body, err := c.Invoke(context.Background(), "lambda://fn?x=1", []byte("p"))
	if err != nil {
		t.Fatalf("lambda://fn?x=1 must be legal now: %v", err)
	}
	if awssdk.ToString(fl.in.FunctionName) != "fn" {
		t.Errorf("FunctionName = %q, want fn (feature params must not leak into the name)",
			awssdk.ToString(fl.in.FunctionName))
	}
	if string(body) != "ok" {
		t.Errorf("body = %q, want ok", body)
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
