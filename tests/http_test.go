package tests

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	awsease "github.com/aura-studio/aws-ease"
)

func newEchoServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Echo-Method", r.Method)
		w.Header().Set("X-Echo-Custom", r.Header.Get("X-Custom"))
		w.Header().Set("X-Echo-Query", r.URL.RawQuery)
		w.Header().Add("X-Multi", "a")
		w.Header().Add("X-Multi", "b")
		if r.URL.Path == "/notfound" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		body, _ := io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
}

func TestHTTPMethodDefaults(t *testing.T) {
	srv := newEchoServer()
	defer srv.Close()
	c := awsease.New()
	ctx := context.Background()

	// 无 Body -> 默认 GET。
	resp, err := c.Do(ctx, srv.URL+"/", nil)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if got := resp.Header.Get("X-Echo-Method"); got != http.MethodGet {
		t.Errorf("no-body method = %q, want GET", got)
	}

	// 有 Body -> 默认 POST，且 Body 被回显。
	resp, err = c.Do(ctx, srv.URL+"/", []byte("hello"))
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if got := resp.Header.Get("X-Echo-Method"); got != http.MethodPost {
		t.Errorf("body method = %q, want POST", got)
	}
	if resp.String() != "hello" {
		t.Errorf("body = %q, want hello", resp.String())
	}
}

func TestHTTPRequestOptions(t *testing.T) {
	srv := newEchoServer()
	defer srv.Close()
	c := awsease.New()

	// 自定义 method/header 走 DoRequest；path/query 写在 Target 里（验证整串保留）。
	resp, err := c.DoRequest(context.Background(), awsease.Request{
		Target: srv.URL + "/x?a=1&b=2",
		Method: http.MethodDelete,
		Header: map[string]string{"X-Custom": "v"},
	})
	if err != nil {
		t.Fatalf("DoRequest: %v", err)
	}
	if got := resp.Header.Get("X-Echo-Method"); got != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", got)
	}
	if got := resp.Header.Get("X-Echo-Custom"); got != "v" {
		t.Errorf("server saw X-Custom = %q, want v", got)
	}
	if got := resp.Header.Get("X-Echo-Query"); got != "a=1&b=2" {
		t.Errorf("query lost: %q", got)
	}
	if got := resp.Header.Values("X-Multi"); len(got) != 2 {
		t.Errorf("multi-value response header lost: %v", got)
	}
	if resp.Requested != srv.URL+"/x?a=1&b=2" {
		t.Errorf("Requested = %q", resp.Requested)
	}
}

func TestHTTPNon2xxNotTransportError(t *testing.T) {
	srv := newEchoServer()
	defer srv.Close()
	c := awsease.New()

	resp, err := c.Do(context.Background(), srv.URL+"/notfound", nil)
	if err != nil {
		t.Fatalf("404 must not be a transport error: %v", err)
	}
	if resp.Status != 404 || resp.OK() {
		t.Errorf("status = %d, OK = %v; want 404 / false", resp.Status, resp.OK())
	}
}
