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
	// 只在成功时写缓存：初始化失败不留任何状态，下次调用整体重试——
	// 瞬态失败（SSO 过期、IMDS 超时、调用方 deadline 太短）不会永久污染客户端。
	awsEndpoint string
	initMu      sync.Mutex
	awsCfg      *awssdk.Config
	lambdaAPI   LambdaAPI
	sqsAPI      SQSAPI

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
// 传 0 或负值表示禁用库级超时、完全交给调用方的 ctx（如 Lambda 同步最长可跑 15 分钟的场景）。
func WithTimeout(d time.Duration) Option {
	return func(c *config) { c.timeout = d }
}

// WithLocalRedirect 把所有 lambda://、sqs:// 调用改写为对 base 的 HTTP mock 请求（本地切换，可选）：
// lambda://<fn> -> {base}/lambda/<fn>，lambda://<fn>/<path> -> {base}/lambda/<fn>/<path>
// （请求体即真实 InvokeInput.Payload：tunnel 模式下是信封字节，同步响应同样拆信封），
// sqs://<q> -> {base}/sqs/<q>，特性参数原样转为重定向 URL 的 query 供 mock 观察。
// 返回值与校验语义和真实后端对齐（sqs/异步成功返回 nil body、sqs 仍校验 UTF-8），
// 本地联调验证过的行为切回真实 AWS 不变。
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

// awsConfigLocked 返回缓存的 aws.Config，必要时加载（调用方须持有 initMu）。
// 注入的 WithAWSConfig 优先；加载失败不写缓存，下次调用重试。
func (c *Client) awsConfigLocked(ctx context.Context) (awssdk.Config, error) {
	if c.awsCfg != nil {
		return *c.awsCfg, nil
	}
	cfg, err := awscfg.LoadDefaultConfig(ctx)
	if err != nil {
		return awssdk.Config{}, err
	}
	c.awsCfg = &cfg
	return cfg, nil
}

// withEndpoint 在设置了 WithAWSEndpoint 时，把自建端点装到 cfg 副本上。
// 用 config 级的 EndpointResolverWithOptions（而非 service Options 的 BaseEndpoint），
// 以兼容下游项目仍在用的 2023 版 aws-sdk-go-v2（v1.18.x，无 BaseEndpoint）。
func (c *Client) withEndpoint(cfg awssdk.Config) awssdk.Config {
	if c.awsEndpoint == "" {
		return cfg
	}
	endpoint := c.awsEndpoint
	cfg.EndpointResolverWithOptions = awssdk.EndpointResolverWithOptionsFunc(
		func(service, region string, options ...interface{}) (awssdk.Endpoint, error) {
			return awssdk.Endpoint{URL: endpoint, HostnameImmutable: true}, nil
		})
	return cfg
}

// lambdaClient 惰性返回 Lambda 客户端（注入优先，否则按 aws.Config 构建，应用 WithAWSEndpoint）。
// 构建成功才缓存；并发调用在初始化期间串行（它们本来也都得等同一份 cfg）。
func (c *Client) lambdaClient(ctx context.Context) (LambdaAPI, error) {
	c.initMu.Lock()
	defer c.initMu.Unlock()
	if c.lambdaAPI != nil {
		return c.lambdaAPI, nil
	}
	cfg, err := c.awsConfigLocked(ctx)
	if err != nil {
		return nil, err
	}
	c.lambdaAPI = awslambda.NewFromConfig(c.withEndpoint(cfg))
	return c.lambdaAPI, nil
}

// sqsClient 惰性返回 SQS 客户端（注入优先，否则按 aws.Config 构建，应用 WithAWSEndpoint）。
// 构建成功才缓存；并发调用在初始化期间串行。
func (c *Client) sqsClient(ctx context.Context) (SQSAPI, error) {
	c.initMu.Lock()
	defer c.initMu.Unlock()
	if c.sqsAPI != nil {
		return c.sqsAPI, nil
	}
	cfg, err := c.awsConfigLocked(ctx)
	if err != nil {
		return nil, err
	}
	c.sqsAPI = awssqs.NewFromConfig(c.withEndpoint(cfg))
	return c.sqsAPI, nil
}
