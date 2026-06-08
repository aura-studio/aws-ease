package lambdatrans

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aura-studio/aws-ease/resolver"
	"github.com/aura-studio/aws-ease/transportcore"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/lambda/types"
)

type LambdaInvoker interface {
	Invoke(ctx context.Context, params *awslambda.InvokeInput, optFns ...func(*awslambda.Options)) (*awslambda.InvokeOutput, error)
}

type Option func(*Transport)

type requestEnvelope struct {
	Path    string          `json:"path"`
	Query   map[string][]string `json:"query"`
	Payload json.RawMessage `json:"payload"`
}

// Transport 通过 Lambda Invoke 执行统一调用。
type Transport struct {
	invoker        LambdaInvoker
	invocationType types.InvocationType
	timeout        time.Duration
	baseEndpoint   string
}

// New 创建 Lambda transport。
func New(opts ...Option) *Transport {
	t := &Transport{
		invocationType: types.InvocationTypeRequestResponse,
		timeout:        30 * time.Second,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(t)
		}
	}
	return t
}

func WithInvoker(invoker LambdaInvoker) Option {
	return func(t *Transport) {
		t.invoker = invoker
	}
}

func WithInvocationType(invocationType types.InvocationType) Option {
	return func(t *Transport) {
		if invocationType != "" {
			t.invocationType = invocationType
		}
	}
}

func WithTimeout(timeout time.Duration) Option {
	return func(t *Transport) {
		if timeout > 0 {
			t.timeout = timeout
		}
	}
}

func WithBaseEndpoint(endpoint string) Option {
	return func(t *Transport) {
		t.baseEndpoint = endpoint
	}
}

// Invoke 将 Endpoint 编码为 Lambda 请求并执行调用。
func (t *Transport) Invoke(ctx context.Context, endpoint resolver.Endpoint, payload []byte) (*transportcore.Response, error) {
	invoker, err := t.ensureInvoker(ctx)
	if err != nil {
		return nil, err
	}

	if len(payload) == 0 {
		payload = []byte("null")
	}
	envelope, err := json.Marshal(requestEnvelope{
		Path:    endpoint.Path,
		Query:   map[string][]string(endpoint.Query),
		Payload: json.RawMessage(payload),
	})
	if err != nil {
		return nil, fmt.Errorf("marshal lambda payload: %w", err)
	}

	invokeCtx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()

	output, err := invoker.Invoke(invokeCtx, &awslambda.InvokeInput{
		FunctionName:   aws.String(endpoint.Host),
		InvocationType: t.invocationType,
		Payload:        envelope,
	})
	if err != nil {
		return nil, fmt.Errorf("invoke lambda %q: %w", endpoint.Host, err)
	}

	response := &transportcore.Response{StatusCode: 200, Body: output.Payload, Headers: map[string]string{}}
	if output.FunctionError != nil {
		response.StatusCode = 500
		response.Headers["Function-Error"] = *output.FunctionError
		return response, fmt.Errorf("lambda function error: %s", *output.FunctionError)
	}

	return response, nil
}

func (t *Transport) ensureInvoker(ctx context.Context) (LambdaInvoker, error) {
	if t.invoker != nil {
		return t.invoker, nil
	}

	loadOptions := []func(*config.LoadOptions) error{}
	if t.baseEndpoint != "" {
		loadOptions = append(loadOptions, config.WithBaseEndpoint(t.baseEndpoint))
	}
	cfg, err := config.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	t.invoker = awslambda.NewFromConfig(cfg)
	return t.invoker, nil
}
