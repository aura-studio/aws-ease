package awsease

import (
	"errors"
	"testing"
)

func TestParseTarget(t *testing.T) {
	tests := []struct {
		name        string
		target      string
		wantBackend Backend
		wantAddr    string
		wantErr     error
	}{
		{"http with path and query", "http://host/path?x=1", BackendHTTP, "http://host/path?x=1", nil},
		{"https keeps whole string", "https://api.svc/v1", BackendHTTP, "https://api.svc/v1", nil},
		{"lambda name", "lambda://order-create", BackendLambda, "order-create", nil},
		{"lambda arn with colons", "lambda://arn:aws:lambda:us-east-1:123:function:fn", BackendLambda, "arn:aws:lambda:us-east-1:123:function:fn", nil},
		{"lambda alias", "lambda://fn:PROD", BackendLambda, "fn:PROD", nil},
		{"sqs name", "sqs://order-events", BackendSQS, "order-events", nil},
		{"sqs fifo name", "sqs://order-events.fifo", BackendSQS, "order-events.fifo", nil},
		{"sqs nested url", "sqs://https://sqs.us-east-1.amazonaws.com/123/q", BackendSQS, "https://sqs.us-east-1.amazonaws.com/123/q", nil},

		{"empty", "", "", "", ErrBadTarget},
		{"no scheme", "order-create", "", "", ErrBadTarget},
		{"empty scheme", "://x", "", "", ErrBadTarget},
		{"http missing host", "http://", "", "", ErrBadTarget},
		{"lambda missing name", "lambda://", "", "", ErrBadTarget},
		{"lambda with path", "lambda://fn/extra", "", "", ErrBadTarget},
		{"lambda with query", "lambda://fn?x=1", "", "", ErrBadTarget},
		{"sqs missing queue", "sqs://", "", "", ErrBadTarget},
		{"unknown scheme", "ftp://host", "", "", ErrUnknownScheme},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend, addr, err := parseTarget(tt.target)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want errors.Is %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if backend != tt.wantBackend {
				t.Errorf("backend = %q, want %q", backend, tt.wantBackend)
			}
			if addr != tt.wantAddr {
				t.Errorf("addr = %q, want %q", addr, tt.wantAddr)
			}
		})
	}
}

func TestRedirectTarget(t *testing.T) {
	t.Run("no base leaves target unchanged", func(t *testing.T) {
		c := New()
		b, addr := c.redirectTarget(BackendLambda, "order-create")
		if b != BackendLambda || addr != "order-create" {
			t.Fatalf("got (%q,%q), want (lambda,order-create)", b, addr)
		}
	})

	c := New(WithLocalRedirect("http://localhost:8080"))
	cases := []struct {
		name        string
		inBackend   Backend
		inAddr      string
		wantBackend Backend
		wantAddr    string
	}{
		{"lambda -> http mock", BackendLambda, "order-create", BackendHTTP, "http://localhost:8080/lambda/order-create"},
		{"sqs -> http mock", BackendSQS, "order-events", BackendHTTP, "http://localhost:8080/sqs/order-events"},
		{"http untouched", BackendHTTP, "http://api/v1", BackendHTTP, "http://api/v1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, addr := c.redirectTarget(tc.inBackend, tc.inAddr)
			if b != tc.wantBackend || addr != tc.wantAddr {
				t.Fatalf("got (%q,%q), want (%q,%q)", b, addr, tc.wantBackend, tc.wantAddr)
			}
		})
	}

	t.Run("trailing slash in base is trimmed", func(t *testing.T) {
		c := New(WithLocalRedirect("http://localhost:8080/"))
		_, addr := c.redirectTarget(BackendSQS, "q")
		if addr != "http://localhost:8080/sqs/q" {
			t.Fatalf("addr = %q, want http://localhost:8080/sqs/q", addr)
		}
	})
}
