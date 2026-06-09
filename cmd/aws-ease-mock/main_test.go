package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEchoBareBody(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(newMux())
	defer srv.Close()

	cases := []struct {
		name        string
		path        string
		wantBackend string
	}{
		{"lambda route", "/lambda/order-create", "lambda"},
		{"sqs route", "/sqs/order-events", "sqs"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Post(srv.URL+tc.path, "application/json", strings.NewReader(`{"hello":"world"}`))
			if err != nil {
				t.Fatalf("post: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200", resp.StatusCode)
			}
			body, _ := io.ReadAll(resp.Body)
			if string(body) != `{"hello":"world"}` {
				t.Errorf("body = %q, want bare echo", string(body))
			}
			if got := resp.Header.Get("X-AWS-Ease-Mock"); got != tc.wantBackend {
				t.Errorf("X-AWS-Ease-Mock = %q, want %q", got, tc.wantBackend)
			}
			if got := resp.Header.Get("X-AWS-Ease-Method"); got != http.MethodPost {
				t.Errorf("X-AWS-Ease-Method = %q, want POST", got)
			}
			if got := resp.Header.Get("X-AWS-Ease-Path"); got != tc.path {
				t.Errorf("X-AWS-Ease-Path = %q, want %q", got, tc.path)
			}
			if got := resp.Header.Get("Content-Type"); got != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", got)
			}
		})
	}
}
