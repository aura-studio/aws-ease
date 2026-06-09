package awsease

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDoHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Echo-Method", r.Method)
		w.Header().Set("X-Echo-Custom", r.Header.Get("X-Custom"))
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
	defer srv.Close()

	c := New()
	ctx := context.Background()

	t.Run("POST with body and header", func(t *testing.T) {
		resp, err := c.doHTTP(ctx, Request{
			Body:   []byte("hello"),
			Header: map[string]string{"X-Custom": "v"},
		}, srv.URL)
		if err != nil {
			t.Fatalf("doHTTP: %v", err)
		}
		if resp.Backend != BackendHTTP {
			t.Errorf("backend = %q", resp.Backend)
		}
		if resp.Status != 200 || !resp.OK() {
			t.Errorf("status = %d, OK = %v", resp.Status, resp.OK())
		}
		if resp.String() != "hello" {
			t.Errorf("body = %q, want hello", resp.String())
		}
		if got := resp.Header.Get("X-Echo-Method"); got != http.MethodPost {
			t.Errorf("server saw method %q, want POST", got)
		}
		if got := resp.Header.Get("X-Echo-Custom"); got != "v" {
			t.Errorf("server saw X-Custom %q, want v", got)
		}
		if got := resp.Header.Values("X-Multi"); len(got) != 2 {
			t.Errorf("multi-value header lost: %v", got)
		}
		if resp.Requested != srv.URL {
			t.Errorf("Requested = %q, want %q", resp.Requested, srv.URL)
		}
	})

	t.Run("empty body defaults to GET", func(t *testing.T) {
		resp, err := c.doHTTP(ctx, Request{}, srv.URL)
		if err != nil {
			t.Fatalf("doHTTP: %v", err)
		}
		if got := resp.Header.Get("X-Echo-Method"); got != http.MethodGet {
			t.Errorf("server saw method %q, want GET", got)
		}
	})

	t.Run("non-2xx is not a transport error", func(t *testing.T) {
		resp, err := c.doHTTP(ctx, Request{}, srv.URL+"/notfound")
		if err != nil {
			t.Fatalf("doHTTP returned transport error for 404: %v", err)
		}
		if resp.Status != 404 || resp.OK() {
			t.Errorf("status = %d, OK = %v; want 404 / false", resp.Status, resp.OK())
		}
	})
}
