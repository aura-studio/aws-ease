package awsease

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
)

func TestDoRequestRoutesByScheme(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Path", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	fl := &fakeLambda{out: &awslambda.InvokeOutput{Payload: []byte(`ok`)}}
	fs := &fakeSQS{sendOut: &awssqs.SendMessageOutput{MessageId: awssdk.String("m")}}
	c := New(WithLambdaAPI(fl), WithSQSAPI(fs))
	ctx := context.Background()

	t.Run("http via Do", func(t *testing.T) {
		resp, err := c.Do(ctx, srv.URL+"/ping", nil)
		if err != nil || resp.Backend != BackendHTTP || resp.Status != 200 {
			t.Fatalf("resp=%+v err=%v", resp, err)
		}
	})
	t.Run("lambda via Do", func(t *testing.T) {
		resp, err := c.Do(ctx, "lambda://order-create", []byte("x"))
		if err != nil || resp.Backend != BackendLambda {
			t.Fatalf("resp=%+v err=%v", resp, err)
		}
	})
	t.Run("sqs via Do", func(t *testing.T) {
		resp, err := c.Do(ctx, "sqs://https://sqs.test/q", []byte("x"))
		if err != nil || resp.Backend != BackendSQS || resp.MessageID != "m" {
			t.Fatalf("resp=%+v err=%v", resp, err)
		}
	})
}

func TestDoRequestParseErrors(t *testing.T) {
	c := New()
	ctx := context.Background()

	if _, err := c.Do(ctx, "not-a-target", nil); !errors.Is(err, ErrBadTarget) {
		t.Errorf("bad target err = %v, want ErrBadTarget", err)
	}
	if _, err := c.Do(ctx, "ftp://host", nil); !errors.Is(err, ErrUnknownScheme) {
		t.Errorf("unknown scheme err = %v, want ErrUnknownScheme", err)
	}
}

// TestDoRequestLocalRedirect 验证 WithLocalRedirect 下 lambda:// 实际走 HTTP mock，
// 且真正的 lambda 客户端不被调用。
func TestDoRequestLocalRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Path", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	fl := &fakeLambda{out: &awslambda.InvokeOutput{Payload: []byte("should-not-run")}}
	c := New(WithLocalRedirect(srv.URL), WithLambdaAPI(fl))

	resp, err := c.Do(context.Background(), "lambda://order-create", []byte("x"))
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if resp.Backend != BackendHTTP {
		t.Errorf("backend = %q, want http (redirected)", resp.Backend)
	}
	if got := resp.Header.Get("X-Path"); got != "/lambda/order-create" {
		t.Errorf("server path = %q, want /lambda/order-create", got)
	}
	if fl.in != nil {
		t.Error("lambda client must not be called when redirected")
	}
}

func TestDoRequestContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done() // 阻塞直到客户端取消/超时
	}))
	defer srv.Close()

	t.Run("cancelled context", func(t *testing.T) {
		c := New()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := c.Do(ctx, srv.URL, nil); err == nil {
			t.Fatal("expected error from cancelled context")
		}
	})

	t.Run("client default timeout fires", func(t *testing.T) {
		c := New(WithTimeout(80 * time.Millisecond))
		_, err := c.Do(context.Background(), srv.URL, nil)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("err = %v, want DeadlineExceeded", err)
		}
	})
}
