package awsease

import (
	"context"
	"net/http"

	"github.com/aura-studio/aws-ease/resolver"
	"github.com/aura-studio/aws-ease/transport/httptrans"
	"github.com/aura-studio/aws-ease/transport/lambdatrans"
	"github.com/aura-studio/aws-ease/transport/sqstrans"
	"github.com/aura-studio/aws-ease/transportcore"
)

// Client 提供开箱即用的统一调用门面。
type Client struct {
	dispatcher *Dispatcher
	resolver   *resolver.Resolver
	httpOpts   []httptrans.Option
}

type ClientOption func(*clientOptions)

type clientOptions struct {
	rewriteRules []rewriteConfig
	httpOpts     []httptrans.Option
	lambdaOpts   []lambdatrans.Option
	sqsOpts      []sqstrans.Option
}

type rewriteConfig struct {
	pattern  string
	template string
}

// New 创建默认注册 http/lambda/sqs transport 的客户端。
func New(opts ...ClientOption) *Client {
	options := &clientOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(options)
		}
	}

	resolverOptions := make([]resolver.Option, 0, len(options.rewriteRules))
	for _, rule := range options.rewriteRules {
		resolverOptions = append(resolverOptions, resolver.Rewrite(rule.pattern, rule.template))
	}
	res := resolver.New(resolverOptions...)

	httpTransport := httptrans.New(options.httpOpts...)
	dispatcher := NewDispatcher(WithResolver(res))
	dispatcher.Register("http", httpTransport)
	dispatcher.Register("https", httpTransport)
	dispatcher.Register("lambda", lambdatrans.New(options.lambdaOpts...))
	dispatcher.Register("sqs", sqstrans.New(options.sqsOpts...))

	return &Client{dispatcher: dispatcher, resolver: res, httpOpts: append([]httptrans.Option(nil), options.httpOpts...)}
}

// WithRewrite 增加一条地址重写规则。
func WithRewrite(pattern, template string) ClientOption {
	return func(options *clientOptions) {
		options.rewriteRules = append(options.rewriteRules, rewriteConfig{pattern: pattern, template: template})
	}
}

// WithHTTPTransportOptions 追加 HTTP transport 配置。
func WithHTTPTransportOptions(opts ...httptrans.Option) ClientOption {
	return func(options *clientOptions) {
		options.httpOpts = append(options.httpOpts, opts...)
	}
}

// WithLambdaTransportOptions 追加 Lambda transport 配置。
func WithLambdaTransportOptions(opts ...lambdatrans.Option) ClientOption {
	return func(options *clientOptions) {
		options.lambdaOpts = append(options.lambdaOpts, opts...)
	}
}

// WithSQSTransportOptions 追加 SQS transport 配置。
func WithSQSTransportOptions(opts ...sqstrans.Option) ClientOption {
	return func(options *clientOptions) {
		options.sqsOpts = append(options.sqsOpts, opts...)
	}
}

// Call 通过 dispatcher 调用目标地址。
func (c *Client) Call(ctx context.Context, target string, payload []byte) (*transportcore.Response, error) {
	return c.dispatcher.Call(ctx, target, payload)
}

// Invoke 是 Call 的别名，适合 Lambda 语义。
func (c *Client) Invoke(ctx context.Context, target string, payload []byte) (*transportcore.Response, error) {
	return c.Call(ctx, target, payload)
}

// Send 是 Call 的别名，适合消息语义。
func (c *Client) Send(ctx context.Context, target string, payload []byte) (*transportcore.Response, error) {
	return c.Call(ctx, target, payload)
}

// Post 使用标准 Call 路径，适合 HTTP POST。
func (c *Client) Post(ctx context.Context, target string, payload []byte) (*transportcore.Response, error) {
	return c.Call(ctx, target, payload)
}

// Get 使用临时 GET transport 调用 HTTP 目标。
func (c *Client) Get(ctx context.Context, target string) (*transportcore.Response, error) {
	endpoint, err := c.resolver.Parse(target)
	if err != nil {
		return nil, err
	}
	transportOpts := append([]httptrans.Option(nil), c.httpOpts...)
	transportOpts = append(transportOpts, httptrans.WithMethod(http.MethodGet))
	transport := httptrans.New(transportOpts...)
	return transport.Invoke(ctx, endpoint, nil)
}
