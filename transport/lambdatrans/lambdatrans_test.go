package lambdatrans

import (
	"context"
	"errors"
	"testing"

	"github.com/aura-studio/aws-ease/resolver"
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/lambda/types"
)

type fakeInvoker struct {
	input  *awslambda.InvokeInput
	output *awslambda.InvokeOutput
	err    error
}

func (f *fakeInvoker) Invoke(_ context.Context, input *awslambda.InvokeInput, _ ...func(*awslambda.Options)) (*awslambda.InvokeOutput, error) {
	f.input = input
	return f.output, f.err
}

func TestLambdaTransportInvokeEncodesEnvelope(t *testing.T) {
	t.Parallel()

	invoker := &fakeInvoker{output: &awslambda.InvokeOutput{Payload: []byte(`{"ok":true}`)}}
	transport := New(WithInvoker(invoker))
	endpoint, err := resolver.Parse("lambda://svc/v1/do?x=1")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	resp, err := transport.Invoke(context.Background(), endpoint, []byte(`{"hello":"world"}`))
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}
	if got := *invoker.input.FunctionName; got != "svc" {
		t.Fatalf("expected function name svc, got %q", got)
	}
	if got := invoker.input.InvocationType; got != types.InvocationTypeRequestResponse {
		t.Fatalf("expected request-response invocation, got %q", got)
	}
	if got := string(invoker.input.Payload); got != `{"path":"/v1/do","query":{"x":["1"]},"payload":{"hello":"world"}}` {
		t.Fatalf("unexpected payload %s", got)
	}
	if string(resp.Body) != `{"ok":true}` {
		t.Fatalf("unexpected response body %q", string(resp.Body))
	}
}

func TestLambdaTransportInvokeReturnsFunctionError(t *testing.T) {
	t.Parallel()

	functionError := "Unhandled"
	invoker := &fakeInvoker{output: &awslambda.InvokeOutput{FunctionError: &functionError, Payload: []byte(`boom`)}}
	transport := New(WithInvoker(invoker))
	endpoint, err := resolver.Parse("lambda://svc/v1/do")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	resp, err := transport.Invoke(context.Background(), endpoint, nil)
	if err == nil {
		t.Fatal("expected function error")
	}
	if resp == nil || string(resp.Body) != "boom" {
		t.Fatalf("expected response body boom, got %#v", resp)
	}
	if resp.StatusCode != 500 {
		t.Fatalf("expected status 500, got %d", resp.StatusCode)
	}
}

func TestLambdaTransportInvokeReturnsInvokerError(t *testing.T) {
	t.Parallel()

	invoker := &fakeInvoker{err: errors.New("network down")}
	transport := New(WithInvoker(invoker))
	endpoint, err := resolver.Parse("lambda://svc/v1/do")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	_, err = transport.Invoke(context.Background(), endpoint, nil)
	if err == nil {
		t.Fatal("expected invoker error")
	}
}
