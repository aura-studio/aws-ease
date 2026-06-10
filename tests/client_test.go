package tests

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"

	awsease "github.com/aura-studio/aws-ease"
)

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// TestWithHTTPClient 验证注入的 *http.Client 被实际使用。
func TestWithHTTPClient(t *testing.T) {
	called := false
	hc := &http.Client{Transport: rtFunc(func(_ *http.Request) (*http.Response, error) {
		called = true
		return &http.Response{
			StatusCode: 201,
			Body:       io.NopCloser(strings.NewReader("custom")),
			Header:     make(http.Header),
		}, nil
	})}
	c := awsease.New(awsease.WithHTTPClient(hc))

	resp, err := c.Do(context.Background(), "http://example.test/", []byte("x"))
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if !called {
		t.Error("injected http client was not used")
	}
	if resp.Status != 201 || resp.String() != "custom" {
		t.Errorf("resp = %+v", resp)
	}
}

// TestWithTimeoutDeadline 验证 WithTimeout 的默认超时会触发（context.DeadlineExceeded）。
func TestWithTimeoutDeadline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done() // 阻塞直到客户端超时/取消
	}))
	defer srv.Close()

	c := awsease.New(awsease.WithTimeout(80 * time.Millisecond))
	_, err := c.Do(context.Background(), srv.URL, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want DeadlineExceeded", err)
	}
}

// TestContextCancel 验证调用方取消 context 会让 Do 返回错误。
func TestContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := awsease.New().Do(ctx, srv.URL, nil); err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

// TestInjectedClientsWithoutAWS 验证注入 mock 后，Lambda/SQS 调用完全不触发任何 AWS 凭证加载。
func TestInjectedClientsWithoutAWS(t *testing.T) {
	fl := &fakeLambda{out: &awslambda.InvokeOutput{Payload: []byte("ok")}}
	fs := &fakeSQS{sendOut: &awssqs.SendMessageOutput{MessageId: awssdk.String("m")}}
	c := awsease.New(awsease.WithLambdaAPI(fl), awsease.WithSQSAPI(fs))
	ctx := context.Background()

	if _, err := c.Do(ctx, "lambda://fn", []byte("x")); err != nil {
		t.Fatalf("lambda Do without AWS: %v", err)
	}
	if _, err := c.Do(ctx, "sqs://https://sqs.test/q", []byte("x")); err != nil {
		t.Fatalf("sqs Do without AWS: %v", err)
	}
	if fl.in == nil || fs.sendIn == nil {
		t.Error("injected clients were not exercised")
	}
}
