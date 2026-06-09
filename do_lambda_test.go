package awsease

import (
	"bytes"
	"context"
	"errors"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdatypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
)

func TestDoLambdaSync(t *testing.T) {
	payload := []byte(`{"sku":"A1","qty":2}`)
	fl := &fakeLambda{out: &awslambda.InvokeOutput{Payload: []byte(`{"id":7}`)}}
	c := New(WithLambdaAPI(fl))

	resp, err := c.doLambda(context.Background(), Request{Body: payload}, "order-create")
	if err != nil {
		t.Fatalf("doLambda: %v", err)
	}

	// 无信封：Payload 必须逐字节等于 Body。
	if !bytes.Equal(fl.in.Payload, payload) {
		t.Errorf("Payload = %q, want %q (envelope leaked?)", fl.in.Payload, payload)
	}
	if awssdk.ToString(fl.in.FunctionName) != "order-create" {
		t.Errorf("FunctionName = %q", awssdk.ToString(fl.in.FunctionName))
	}
	if fl.in.InvocationType != lambdatypes.InvocationTypeRequestResponse {
		t.Errorf("InvocationType = %q, want RequestResponse", fl.in.InvocationType)
	}
	if resp.Backend != BackendLambda || resp.Status != 0 || resp.FuncError != "" || !resp.OK() {
		t.Errorf("resp = %+v; want lambda/status0/no-funcerr/ok", resp)
	}
	if resp.String() != `{"id":7}` {
		t.Errorf("body = %q", resp.String())
	}
	if resp.Requested != "order-create" {
		t.Errorf("Requested = %q", resp.Requested)
	}
}

func TestDoLambdaFuncError(t *testing.T) {
	fl := &fakeLambda{out: &awslambda.InvokeOutput{
		Payload:       []byte(`{"errorMessage":"boom"}`),
		FunctionError: awssdk.String("Unhandled"),
	}}
	c := New(WithLambdaAPI(fl))

	resp, err := c.doLambda(context.Background(), Request{Body: []byte("{}")}, "order-create")
	if err != nil {
		t.Fatalf("func error must not be a transport error, got: %v", err)
	}
	if resp.FuncError != "Unhandled" || resp.OK() || resp.Status != 0 {
		t.Errorf("resp = %+v; want FuncError set, OK false, status 0", resp)
	}
	if resp.String() != `{"errorMessage":"boom"}` {
		t.Errorf("error payload not surfaced: %q", resp.String())
	}
}

func TestDoLambdaAsync(t *testing.T) {
	fl := &fakeLambda{out: &awslambda.InvokeOutput{}}
	c := New(WithLambdaAPI(fl))

	resp, err := c.doLambda(context.Background(), Request{Body: []byte("x"), Async: true}, "audit-logger")
	if err != nil {
		t.Fatalf("doLambda: %v", err)
	}
	if fl.in.InvocationType != lambdatypes.InvocationTypeEvent {
		t.Errorf("InvocationType = %q, want Event", fl.in.InvocationType)
	}
	if !resp.Async || resp.Body != nil || !resp.OK() {
		t.Errorf("resp = %+v; want Async true, Body nil, OK true", resp)
	}
}

func TestDoLambdaTransportError(t *testing.T) {
	fl := &fakeLambda{err: errors.New("network down")}
	c := New(WithLambdaAPI(fl))

	resp, err := c.doLambda(context.Background(), Request{Body: []byte("x")}, "fn")
	if err == nil {
		t.Fatal("expected transport error")
	}
	if resp != nil {
		t.Errorf("resp must be nil on transport error, got %+v", resp)
	}
}
