package awsease

import (
	"context"
	"testing"

	"github.com/aura-studio/aws-ease/resolver"
)

type fakeTransport struct {
	called  bool
	last    Endpoint
	lastPay []byte
	resp    *Response
	err     error
}

func (f *fakeTransport) Invoke(_ context.Context, endpoint Endpoint, payload []byte) (*Response, error) {
	f.called = true
	f.last = endpoint
	f.lastPay = append([]byte(nil), payload...)
	return f.resp, f.err
}

func TestDispatcherCallRoutesByScheme(t *testing.T) {
	t.Parallel()

	dispatcher := NewDispatcher()
	fake := &fakeTransport{resp: &Response{StatusCode: 202, Body: []byte("ok")}}
	dispatcher.Register("lambda", fake)

	resp, err := dispatcher.Call(context.Background(), "lambda://svc/do?x=1", []byte("payload"))
	if err != nil {
		t.Fatalf("Call returned error: %v", err)
	}
	if !fake.called {
		t.Fatal("expected lambda transport to be called")
	}
	if fake.last.Scheme != "lambda" || fake.last.Host != "svc" || fake.last.Path != "/do" {
		t.Fatalf("unexpected endpoint routed: %#v", fake.last)
	}
	if string(fake.lastPay) != "payload" {
		t.Fatalf("unexpected payload: %q", string(fake.lastPay))
	}
	if resp.StatusCode != 202 || string(resp.Body) != "ok" {
		t.Fatalf("unexpected response: %#v", resp)
	}
}

func TestDispatcherCallReturnsErrorForUnregisteredScheme(t *testing.T) {
	t.Parallel()

	dispatcher := NewDispatcher()

	_, err := dispatcher.Call(context.Background(), "lambda://svc/do", nil)
	if err == nil {
		t.Fatal("expected error for unregistered scheme")
	}
}

func TestDispatcherCallUsesResolverRewrite(t *testing.T) {
	t.Parallel()

	res := resolver.New(
		resolver.Rewrite("lambda://*", "http://localhost:8080/{host}{path}?{query}"),
	)
	dispatcher := NewDispatcher(WithResolver(res))
	httpTransport := &fakeTransport{resp: &Response{StatusCode: 200, Body: []byte("rewritten")}}
	dispatcher.Register("http", httpTransport)

	resp, err := dispatcher.Call(context.Background(), "lambda://svc/do?x=1", []byte("payload"))
	if err != nil {
		t.Fatalf("Call returned error: %v", err)
	}
	if !httpTransport.called {
		t.Fatal("expected http transport to be called after rewrite")
	}
	if httpTransport.last.Scheme != "http" || httpTransport.last.Host != "localhost:8080" || httpTransport.last.Path != "/svc/do" {
		t.Fatalf("unexpected rewritten endpoint: %#v", httpTransport.last)
	}
	if httpTransport.last.Query.Get("x") != "1" {
		t.Fatalf("unexpected rewritten query: %#v", httpTransport.last.Query)
	}
	if string(resp.Body) != "rewritten" {
		t.Fatalf("unexpected response body: %q", string(resp.Body))
	}
}
