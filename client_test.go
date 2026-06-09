package awsease

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestNewDefaults(t *testing.T) {
	c := New()
	if c.httpClient == nil {
		t.Fatal("httpClient should be non-nil")
	}
	if c.timeout != defaultTimeout {
		t.Fatalf("timeout = %v, want %v", c.timeout, defaultTimeout)
	}
	if c.queueURLs == nil {
		t.Fatal("queueURLs cache should be initialized")
	}
}

func TestOptionsApply(t *testing.T) {
	hc := &http.Client{}
	c := New(
		WithHTTPClient(hc),
		WithTimeout(5*time.Second),
		WithLocalRedirect("http://localhost:9000"),
		WithAWSEndpoint("http://localhost:4566"),
	)
	if c.httpClient != hc {
		t.Error("WithHTTPClient not applied")
	}
	if c.timeout != 5*time.Second {
		t.Errorf("timeout = %v, want 5s", c.timeout)
	}
	if c.redirectBase != "http://localhost:9000" {
		t.Errorf("redirectBase = %q", c.redirectBase)
	}
	if c.awsEndpoint != "http://localhost:4566" {
		t.Errorf("awsEndpoint = %q", c.awsEndpoint)
	}
}

// TestLazyClientsReturnInjectedWithoutAWS 验证注入 mock 后，惰性获取不触发任何 AWS 凭证加载
// （LoadDefaultConfig 永不被调用，cfgOnce 不执行）。
func TestLazyClientsReturnInjectedWithoutAWS(t *testing.T) {
	fl := &fakeLambda{}
	fs := &fakeSQS{}
	c := New(WithLambdaAPI(fl), WithSQSAPI(fs))

	gotL, err := c.lambdaClient(context.Background())
	if err != nil {
		t.Fatalf("lambdaClient: %v", err)
	}
	if gotL != LambdaAPI(fl) {
		t.Error("expected injected lambda client")
	}

	gotS, err := c.sqsClient(context.Background())
	if err != nil {
		t.Fatalf("sqsClient: %v", err)
	}
	if gotS != SQSAPI(fs) {
		t.Error("expected injected sqs client")
	}
}
