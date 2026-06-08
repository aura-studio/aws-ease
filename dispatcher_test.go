package awsease

import (
	"context"
	"testing"
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
