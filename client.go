package awsease

import (
	"context"
	"net/http"
	"sync"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
)

const defaultTimeout = 30 * time.Second

// LambdaAPI 是 doLambda 依赖的最小 Lambda 客户端接口（签名即 aws-sdk-go-v2 原生形状，便于 mock）。
type LambdaAPI interface {
	Invoke(ctx context.Context, in *awslambda.InvokeInput, optFns ...func(*awslambda.Options)) (*awslambda.InvokeOutput, error)
}

// SQSAPI 是 doSQS 依赖的最小 SQS 客户端接口（签名即 aws-sdk-go-v2 原生形状，便于 mock）。
type SQSAPI interface {
	SendMessage(ctx context.Context, in *awssqs.SendMessageInput, optFns ...func(*awssqs.Options)) (*awssqs.SendMessageOutput, error)
	GetQueueUrl(ctx context.Context, in *awssqs.GetQueueUrlInput, optFns ...func(*awssqs.Options)) (*awssqs.GetQueueUrlOutput, error)
}

// config 是 New 期间收集 Option 的临时载体。
type config struct {
	httpClient   *http.Client
	awsCfg       *awssdk.Config
	lambdaAPI    LambdaAPI
	sqsAPI       SQSAPI
	awsEndpoint  string
	timeout      time.Duration
	redirectBase string
}

// Client 是唯一门面，并发安全（含 SQS QueueUrl 缓存的内部加锁）。零值不可用，必须经 New 构造。
type Client struct {
	httpClient   *http.Client
	timeout      time.Duration
	redirectBase string

	// AWS 相关：真实客户端惰性加载，只用 HTTP 时不触发任何凭证读取。
	awsEndpoint string
	awsCfg      *awssdk.Config
	lambdaAPI   LambdaAPI
	sqsAPI      SQSAPI
	cfgOnce     sync.Once
	lambdaOnce  sync.Once
	sqsOnce     sync.Once
	initErr     error

	// SQS 队列名 -> QueueUrl 缓存。
	mu        sync.RWMutex
	queueURLs map[string]string
}

// Option 配置 Client（构造期、基础设施级，构造一次）。
type Option func(*config)

// WithHTTPClient 替换底层 *http.Client（transport / 代理 / mTLS / 连接池）。
// 注意：超时请用 context 或 WithTimeout，注入的 http.Client.Timeout 应留零，避免与 context 超时打架。
func WithHTTPClient(h *http.Client) Option {
	return func(c *config) {
		if h != nil {
			c.httpClient = h
		}
	}
}

// WithAWSConfig 注入已加载的 aws.Config，共享给 lambda + sqs（生产最常用，一次加载）。
func WithAWSConfig(cfg awssdk.Config) Option {
	return func(c *config) { c.awsCfg = &cfg }
}

// WithLambdaAPI 注入 Lambda 客户端实现（测试 mock）。
func WithLambdaAPI(l LambdaAPI) Option {
	return func(c *config) { c.lambdaAPI = l }
}

// WithSQSAPI 注入 SQS 客户端实现（测试 mock）。
func WithSQSAPI(s SQSAPI) Option {
	return func(c *config) { c.sqsAPI = s }
}

// WithAWSEndpoint 为 lambda/sqs 设置 base endpoint（指向 LocalStack 等自建端点）。
func WithAWSEndpoint(url string) Option {
	return func(c *config) { c.awsEndpoint = url }
}

// WithTimeout 设置每次调用的默认超时（内部以 context.WithTimeout 套在传入 ctx 上）。默认 30s。
func WithTimeout(d time.Duration) Option {
	return func(c *config) {
		if d > 0 {
			c.timeout = d
		}
	}
}

// WithLocalRedirect 把所有 lambda://、sqs:// 调用改写为对 base 的 HTTP mock 请求（本地切换，可选）。
func WithLocalRedirect(base string) Option {
	return func(c *config) { c.redirectBase = base }
}

// New 创建客户端。无参即可用（HTTP 立即可用；lambda/sqs 的真实 AWS 客户端惰性加载）。
func New(opts ...Option) *Client {
	cfg := &config{
		httpClient: &http.Client{},
		timeout:    defaultTimeout,
	}
	for _, o := range opts {
		if o != nil {
			o(cfg)
		}
	}
	return &Client{
		httpClient:   cfg.httpClient,
		timeout:      cfg.timeout,
		redirectBase: cfg.redirectBase,
		awsEndpoint:  cfg.awsEndpoint,
		awsCfg:       cfg.awsCfg,
		lambdaAPI:    cfg.lambdaAPI,
		sqsAPI:       cfg.sqsAPI,
		queueURLs:    map[string]string{},
	}
}

// awsConfig 惰性加载并缓存 aws.Config（注入的 WithAWSConfig 优先，否则 LoadDefaultConfig 一次）。
func (c *Client) awsConfig(ctx context.Context) (awssdk.Config, error) {
	c.cfgOnce.Do(func() {
		if c.awsCfg != nil {
			return // 已注入
		}
		cfg, err := awscfg.LoadDefaultConfig(ctx)
		if err != nil {
			c.initErr = err
			return
		}
		c.awsCfg = &cfg
	})
	if c.awsCfg == nil {
		return awssdk.Config{}, c.initErr
	}
	return *c.awsCfg, nil
}

// lambdaClient 惰性返回 Lambda 客户端（注入优先，否则按 aws.Config 构建，应用 WithAWSEndpoint）。
func (c *Client) lambdaClient(ctx context.Context) (LambdaAPI, error) {
	c.lambdaOnce.Do(func() {
		if c.lambdaAPI != nil {
			return // 已注入
		}
		cfg, err := c.awsConfig(ctx)
		if err != nil {
			return
		}
		c.lambdaAPI = awslambda.NewFromConfig(cfg, func(o *awslambda.Options) {
			if c.awsEndpoint != "" {
				o.BaseEndpoint = awssdk.String(c.awsEndpoint)
			}
		})
	})
	if c.lambdaAPI == nil {
		return nil, c.initErr
	}
	return c.lambdaAPI, nil
}

// sqsClient 惰性返回 SQS 客户端（注入优先，否则按 aws.Config 构建，应用 WithAWSEndpoint）。
func (c *Client) sqsClient(ctx context.Context) (SQSAPI, error) {
	c.sqsOnce.Do(func() {
		if c.sqsAPI != nil {
			return // 已注入
		}
		cfg, err := c.awsConfig(ctx)
		if err != nil {
			return
		}
		c.sqsAPI = awssqs.NewFromConfig(cfg, func(o *awssqs.Options) {
			if c.awsEndpoint != "" {
				o.BaseEndpoint = awssdk.String(c.awsEndpoint)
			}
		})
	})
	if c.sqsAPI == nil {
		return nil, c.initErr
	}
	return c.sqsAPI, nil
}
