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

// TestParseErrorsViaDo 通过 Do 的可观察错误覆盖地址解析的各类非法输入
// （解析在分发前完成，故无需任何后端客户端）。
func TestParseErrorsViaDo(t *testing.T) {
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
		{"lambda with query", "lambda://fn?x=1", awsease.ErrBadTarget},
		{"sqs missing queue", "sqs://", awsease.ErrBadTarget},
		{"unknown scheme", "ftp://host", awsease.ErrUnknownScheme},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := c.Do(ctx, tt.target, nil)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want errors.Is %v", err, tt.want)
			}
			if resp != nil {
				t.Errorf("resp must be nil on parse error, got %+v", resp)
			}
		})
	}
}

// TestRoutesByScheme 验证 scheme 决定后端：http/lambda/sqs 各自路由到正确后端。
func TestRoutesByScheme(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	fl := &fakeLambda{out: &awslambda.InvokeOutput{Payload: []byte("ok")}}
	fs := &fakeSQS{sendOut: &awssqs.SendMessageOutput{MessageId: awssdk.String("m")}}
	c := awsease.New(awsease.WithLambdaAPI(fl), awsease.WithSQSAPI(fs))
	ctx := context.Background()

	if resp, err := c.Do(ctx, srv.URL+"/ping", nil); err != nil || resp.Backend != awsease.BackendHTTP {
		t.Fatalf("http: resp=%+v err=%v", resp, err)
	}
	if resp, err := c.Do(ctx, "lambda://order-create", []byte("x")); err != nil || resp.Backend != awsease.BackendLambda {
		t.Fatalf("lambda: resp=%+v err=%v", resp, err)
	}
	if resp, err := c.Do(ctx, "sqs://https://sqs.test/q", []byte("x")); err != nil || resp.Backend != awsease.BackendSQS || resp.MessageID != "m" {
		t.Fatalf("sqs: resp=%+v err=%v", resp, err)
	}
}

// TestLocalRedirect 验证 WithLocalRedirect 下 lambda:// 实际走 HTTP mock，且 lambda 客户端不被调用。
func TestLocalRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Path", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	fl := &fakeLambda{out: &awslambda.InvokeOutput{Payload: []byte("should-not-run")}}
	c := awsease.New(awsease.WithLocalRedirect(srv.URL), awsease.WithLambdaAPI(fl))

	resp, err := c.Do(context.Background(), "lambda://order-create", []byte("x"))
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if resp.Backend != awsease.BackendHTTP {
		t.Errorf("backend = %q, want http (redirected)", resp.Backend)
	}
	if got := resp.Header.Get("X-Path"); got != "/lambda/order-create" {
		t.Errorf("server path = %q, want /lambda/order-create", got)
	}
	if fl.in != nil {
		t.Error("lambda client must not be called when redirected")
	}
}
