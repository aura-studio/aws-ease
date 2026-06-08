package httptrans

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aura-studio/aws-ease/resolver"
)

func TestHTTPTransportInvoke(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("expected PUT method, got %s", r.Method)
		}
		if r.URL.Path != "/v1/do" {
			t.Fatalf("expected path /v1/do, got %s", r.URL.Path)
		}
		if r.URL.RawQuery != "x=1" {
			t.Fatalf("expected query x=1, got %s", r.URL.RawQuery)
		}
		if r.Header.Get("X-Test") != "ok" {
			t.Fatalf("expected X-Test header, got %q", r.Header.Get("X-Test"))
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if string(body) != "payload" {
			t.Fatalf("expected payload body, got %q", string(body))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	transport := New(
		WithMethod(http.MethodPut),
		WithHeader("X-Test", "ok"),
		WithTimeout(2*time.Second),
	)

	endpoint, err := resolver.Parse(server.URL + "/v1/do?x=1")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	resp, err := transport.Invoke(context.Background(), endpoint, []byte("payload"))
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", resp.StatusCode)
	}
	if string(resp.Body) != `{"ok":true}` {
		t.Fatalf("unexpected body %q", string(resp.Body))
	}
	if resp.Headers["Content-Type"] != "application/json" {
		t.Fatalf("unexpected content-type %q", resp.Headers["Content-Type"])
	}
}
