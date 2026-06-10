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
// 非 2xx 在新 API 下是 err，所以这里回放 200 来验证注入生效 + 返回体正确。
func TestWithHTTPClient(t *testing.T) {
	called := false
	hc := &http.Client{Transport: rtFunc(func(_ *http.Request) (*http.Response, error) {
		called = true
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("custom")),
			Header:     make(http.Header),
		}, nil
	})}
	c := awsease.New(awsease.WithHTTPClient(hc))

	body, err := c.Invoke(context.Background(), "http://example.test/", []byte("x"))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !called {
		t.Error("injected http client was not used")
	}
	if string(body) != "custom" {
		t.Errorf("body = %q, want custom", body)
	}
}

// TestWithTimeoutDeadline 验证 WithTimeout 的默认超时会触发（context.DeadlineExceeded）。
func TestWithTimeoutDeadline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done() // 阻塞直到客户端超时/取消
	}))
	defer srv.Close()

	c := awsease.New(awsease.WithTimeout(80 * time.Millisecond))
	body, err := c.Invoke(context.Background(), srv.URL, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want DeadlineExceeded", err)
	}
	if body != nil {
		t.Errorf("body must be nil on timeout, got %q", body)
	}
}

// TestWithTimeoutDisabled 验证 WithTimeout(0)（及负值）禁用库级默认超时：注入 RoundTripper
// 观察请求 ctx 的 Deadline()——传入的 ctx 自身无 deadline，请求里若出现 deadline 只可能来自
// 库级 WithTimeout。默认 New() 必须有（30s 兜底），显式传 0/负值后必须没有（完全交还调用方
// ctx，覆盖 Lambda 同步最长可跑 15 分钟这类超过默认值的场景）。
func TestWithTimeoutDisabled(t *testing.T) {
	tests := []struct {
		name         string
		extra        []awsease.Option
		wantDeadline bool
	}{
		{"default has deadline", nil, true},
		{"zero disables deadline", []awsease.Option{awsease.WithTimeout(0)}, false},
		{"negative disables deadline", []awsease.Option{awsease.WithTimeout(-time.Second)}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotDeadline bool
			hc := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
				_, gotDeadline = r.Context().Deadline()
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("ok")),
					Header:     make(http.Header),
				}, nil
			})}
			c := awsease.New(append([]awsease.Option{awsease.WithHTTPClient(hc)}, tt.extra...)...)

			if _, err := c.Invoke(context.Background(), "http://example.test/", nil); err != nil {
				t.Fatalf("Invoke: %v", err)
			}
			if gotDeadline != tt.wantDeadline {
				t.Errorf("request deadline present = %v, want %v", gotDeadline, tt.wantDeadline)
			}
		})
	}
}

// TestContextCancel 验证调用方取消 context 会让 Invoke 返回错误。
func TestContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := awsease.New().Invoke(ctx, srv.URL, nil); err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

// TestInjectedClientsWithoutAWS 验证注入 mock 后，Lambda/SQS 调用完全不触发任何 AWS 凭证加载。
func TestInjectedClientsWithoutAWS(t *testing.T) {
	fl := &fakeLambda{out: &awslambda.InvokeOutput{Payload: []byte("ok")}}
	fs := &fakeSQS{sendOut: &awssqs.SendMessageOutput{MessageId: awssdk.String("m")}}
	c := awsease.New(awsease.WithLambdaAPI(fl), awsease.WithSQSAPI(fs))
	ctx := context.Background()

	if _, err := c.Invoke(ctx, "lambda://fn", []byte("x")); err != nil {
		t.Fatalf("lambda Invoke without AWS: %v", err)
	}
	if _, err := c.Invoke(ctx, "sqs://https://sqs.test/q", []byte("x")); err != nil {
		t.Fatalf("sqs Invoke without AWS: %v", err)
	}
	if fl.in == nil || fs.sendIn == nil {
		t.Error("injected clients were not exercised")
	}
}
