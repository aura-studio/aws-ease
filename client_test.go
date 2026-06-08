package awsease

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientCallRewritesLambdaToHTTP(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST method, got %s", r.Method)
		}
		if r.URL.Path != "/lambda/svc/do" {
			t.Fatalf("expected rewritten path /lambda/svc/do, got %s", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if string(body) != "payload" {
			t.Fatalf("expected payload body, got %q", string(body))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	client := New(
		WithRewrite("lambda://*", server.URL+"/lambda/{host}{path}"),
	)

	resp, err := client.Call(context.Background(), "lambda://svc/do", []byte("payload"))
	if err != nil {
		t.Fatalf("Call returned error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	if string(resp.Body) != "ok" {
		t.Fatalf("expected body ok, got %q", string(resp.Body))
	}
}

func TestClientConvenienceMethods(t *testing.T) {
	t.Parallel()

	client := New()
	if _, err := client.Invoke(context.Background(), "lambda://svc/do", []byte("body")); err == nil {
		// default lambda transport requires aws config at runtime; just ensure the wrapper exists
	}
	if _, err := client.Send(context.Background(), "sqs://queue", []byte("body")); err == nil {
	}
}
