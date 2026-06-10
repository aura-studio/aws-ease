package tests

import (
	"bytes"
	"context"
	"errors"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdatypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"

	awsease "github.com/aura-studio/aws-ease"
)

func TestLambdaSyncNoEnvelope(t *testing.T) {
	payload := []byte(`{"sku":"A1","qty":2}`)
	fl := &fakeLambda{out: &awslambda.InvokeOutput{Payload: []byte(`{"id":7}`)}}
	c := awsease.New(awsease.WithLambdaAPI(fl))

	resp, err := c.Do(context.Background(), "lambda://order-create", payload)
	if err != nil {
		t.Fatalf("Do: %v", err)
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
	if resp.Backend != awsease.BackendLambda || resp.Status != 0 || resp.FuncError != "" || !resp.OK() {
		t.Errorf("resp = %+v", resp)
	}
	if resp.String() != `{"id":7}` || resp.Requested != "order-create" {
		t.Errorf("body=%q requested=%q", resp.String(), resp.Requested)
	}
}

func TestLambdaArnFunctionName(t *testing.T) {
	arn := "arn:aws:lambda:us-east-1:123:function:order-create"
	fl := &fakeLambda{out: &awslambda.InvokeOutput{}}
	c := awsease.New(awsease.WithLambdaAPI(fl))

	if _, err := c.Do(context.Background(), "lambda://"+arn, []byte("{}")); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if awssdk.ToString(fl.in.FunctionName) != arn {
		t.Errorf("FunctionName = %q, want %q", awssdk.ToString(fl.in.FunctionName), arn)
	}
}

func TestLambdaFuncError(t *testing.T) {
	fl := &fakeLambda{out: &awslambda.InvokeOutput{
		Payload:       []byte(`{"errorMessage":"boom"}`),
		FunctionError: awssdk.String("Unhandled"),
	}}
	c := awsease.New(awsease.WithLambdaAPI(fl))

	resp, err := c.Do(context.Background(), "lambda://order-create", []byte("{}"))
	if err != nil {
		t.Fatalf("func error must not be a transport error: %v", err)
	}
	if resp.FuncError != "Unhandled" || resp.OK() || resp.Status != 0 {
		t.Errorf("resp = %+v; want FuncError set, OK false, status 0", resp)
	}
	if resp.String() != `{"errorMessage":"boom"}` {
		t.Errorf("error payload not surfaced: %q", resp.String())
	}
}

func TestLambdaAsync(t *testing.T) {
	fl := &fakeLambda{out: &awslambda.InvokeOutput{}}
	c := awsease.New(awsease.WithLambdaAPI(fl))

	resp, err := c.DoRequest(context.Background(), awsease.Request{
		Target: "lambda://audit-logger",
		Body:   []byte("x"),
		Async:  true,
	})
	if err != nil {
		t.Fatalf("DoRequest: %v", err)
	}
	if fl.in.InvocationType != lambdatypes.InvocationTypeEvent {
		t.Errorf("InvocationType = %q, want Event", fl.in.InvocationType)
	}
	if !resp.Async || resp.Body != nil || !resp.OK() {
		t.Errorf("resp = %+v; want Async true, Body nil, OK true", resp)
	}
}

func TestLambdaTransportError(t *testing.T) {
	fl := &fakeLambda{err: errors.New("network down")}
	c := awsease.New(awsease.WithLambdaAPI(fl))

	resp, err := c.Do(context.Background(), "lambda://fn", []byte("x"))
	if err == nil {
		t.Fatal("expected transport error")
	}
	if resp != nil {
		t.Errorf("resp must be nil on transport error, got %+v", resp)
	}
}
