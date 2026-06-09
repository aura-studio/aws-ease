// Package awsease 提供对 HTTP / AWS Lambda / AWS SQS 的统一、便捷封装。
//
// 心智模型只有「地址 + Body -> Response」：
//   - 地址是 backend://target 字符串，scheme 决定后端（http/https/lambda/sqs）。
//   - Response 用 Backend 标签 + 后端专属字段诚实区分三种语义，绝不给非 HTTP 后端伪造状态码。
//
// 主入口是 Client.Do(ctx, target, body)；需要细控时用 Client.DoRequest(ctx, Request)。
// 本地/生产切换靠换地址串，或 New(WithLocalRedirect(base))。
//
// 设计规格见 doc/plan.md。
package awsease

// Version 是当前模块语义化版本号。
const Version = "0.2.0"

// Backend 是后端类型，三选一。它是 Response 的「自解释标签」，由地址 scheme 推导。
type Backend string

const (
	// BackendHTTP 表示 http:// 或 https://，标准 HTTP 请求/响应。
	BackendHTTP Backend = "http"
	// BackendLambda 表示 lambda://<fn>，lambda.Invoke（同步取 payload，或异步 Event 即发即忘）。
	BackendLambda Backend = "lambda"
	// BackendSQS 表示 sqs://<queue>，sqs.SendMessage（推送取 MessageId）。
	BackendSQS Backend = "sqs"
)
