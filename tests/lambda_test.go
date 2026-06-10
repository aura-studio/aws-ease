package tests

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdatypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"

	awsease "github.com/aura-studio/aws-ease"
)

// TestLambdaSyncNoEnvelope 验证同步调用无信封：发出与收回的 payload 都逐字节透传。
func TestLambdaSyncNoEnvelope(t *testing.T) {
	payload := []byte(`{"sku":"A1","qty":2}`)
	fl := &fakeLambda{out: &awslambda.InvokeOutput{Payload: []byte(`{"id":7}`)}}
	c := awsease.New(awsease.WithLambdaAPI(fl))

	body, err := c.Invoke(context.Background(), "lambda://order-create", payload)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	// 无信封：发出的 Payload 必须逐字节等于传入 payload。
	if !bytes.Equal(fl.in.Payload, payload) {
		t.Errorf("Payload = %q, want %q (envelope leaked?)", fl.in.Payload, payload)
	}
	if awssdk.ToString(fl.in.FunctionName) != "order-create" {
		t.Errorf("FunctionName = %q", awssdk.ToString(fl.in.FunctionName))
	}
	if fl.in.InvocationType != lambdatypes.InvocationTypeRequestResponse {
		t.Errorf("InvocationType = %q, want RequestResponse", fl.in.InvocationType)
	}
	// 同步成功返回 lambda 的返回 payload 字节，同样无信封。
	if !bytes.Equal(body, []byte(`{"id":7}`)) {
		t.Errorf("body = %q, want %q", body, `{"id":7}`)
	}
}

// TestLambdaArnFunctionName 验证完整 ARN 也能原样落到 FunctionName（ARN 含 ":" 不含 "/"，合法）。
func TestLambdaArnFunctionName(t *testing.T) {
	arn := "arn:aws:lambda:us-east-1:123:function:order-create"
	fl := &fakeLambda{out: &awslambda.InvokeOutput{}}
	c := awsease.New(awsease.WithLambdaAPI(fl))

	if _, err := c.Invoke(context.Background(), "lambda://"+arn, []byte("{}")); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if awssdk.ToString(fl.in.FunctionName) != arn {
		t.Errorf("FunctionName = %q, want %q", awssdk.ToString(fl.in.FunctionName), arn)
	}
}

// TestLambdaFunctionError 验证函数内部错误（FunctionError 非空）走 err：
// body 恒为 nil，错误名与错误 payload 都进错误串。
func TestLambdaFunctionError(t *testing.T) {
	fl := &fakeLambda{out: &awslambda.InvokeOutput{
		Payload:       []byte(`{"errorMessage":"boom"}`),
		FunctionError: awssdk.String("Unhandled"),
	}}
	c := awsease.New(awsease.WithLambdaAPI(fl))

	body, err := c.Invoke(context.Background(), "lambda://order-create", []byte("{}"))
	if err == nil {
		t.Fatal("function error must surface as err")
	}
	if body != nil {
		t.Errorf("body must be nil on function error, got %q", body)
	}
	if !strings.Contains(err.Error(), "Unhandled") {
		t.Errorf("err %q must contain the function error name", err)
	}
	if !strings.Contains(err.Error(), `{"errorMessage":"boom"}`) {
		t.Errorf("err %q must carry the error payload", err)
	}
}

// TestLambdaAsyncValues 表驱动覆盖 async 的取值边界：只有 "true" 与 "1" 触发即发即忘
// （InvocationType=Event、返回 (nil, nil)）；"false"、"0"、缺省都按同步处理并回放返回 payload。
// 每个 case 用独立 fakeLambda：入参记录互不残留，断言到的一定是本 case 的调用。
func TestLambdaAsyncValues(t *testing.T) {
	replay := []byte(`{"id":7}`)
	tests := []struct {
		name     string
		target   string
		wantType lambdatypes.InvocationType
		wantBody []byte // nil 表示即发即忘：不返回任何 body
	}{
		{"async=true", "lambda://audit-logger?async=true", lambdatypes.InvocationTypeEvent, nil},
		{"async=1", "lambda://audit-logger?async=1", lambdatypes.InvocationTypeEvent, nil},
		{"async=false", "lambda://audit-logger?async=false", lambdatypes.InvocationTypeRequestResponse, replay},
		{"async=0", "lambda://audit-logger?async=0", lambdatypes.InvocationTypeRequestResponse, replay},
		{"no param", "lambda://audit-logger", lambdatypes.InvocationTypeRequestResponse, replay},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// fake 始终回放一份 payload：异步路径若误把它返回，wantBody=nil 的断言会立刻暴露。
			fl := &fakeLambda{out: &awslambda.InvokeOutput{Payload: replay}}
			c := awsease.New(awsease.WithLambdaAPI(fl))

			body, err := c.Invoke(context.Background(), tt.target, []byte("x"))
			if err != nil {
				t.Fatalf("Invoke: %v", err)
			}
			if fl.in.InvocationType != tt.wantType {
				t.Errorf("InvocationType = %q, want %q", fl.in.InvocationType, tt.wantType)
			}
			if tt.wantBody == nil {
				if body != nil {
					t.Errorf("fire-and-forget body = %q, want nil", body)
				}
				return
			}
			if !bytes.Equal(body, tt.wantBody) {
				t.Errorf("body = %q, want %q", body, tt.wantBody)
			}
		})
	}
}

// TestLambdaTransportError 验证传输层错误透传：err 非 nil 且 body 恒为 nil。
func TestLambdaTransportError(t *testing.T) {
	fl := &fakeLambda{err: errors.New("network down")}
	c := awsease.New(awsease.WithLambdaAPI(fl))

	body, err := c.Invoke(context.Background(), "lambda://fn", []byte("x"))
	if err == nil {
		t.Fatal("expected transport error")
	}
	if body != nil {
		t.Errorf("body must be nil on transport error, got %q", body)
	}
}
