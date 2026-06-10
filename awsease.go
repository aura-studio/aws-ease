// Package awsease 提供对 HTTP / AWS Lambda / AWS SQS 的统一、便捷封装。
//
// 心智模型只有一行：「Invoke(ctx, url, payload) -> (body, err)」。
//   - url 是 backend://target 字符串，scheme 决定后端（http/https/lambda/sqs）；
//     特性参数（Lambda 异步、SQS 属性/FIFO）以 query 参数写在 url 里；HTTP 后端无特性参数。
//   - payload 与返回值都只是有效数据本身，原样透传，库不包任何信封。
//   - 一切失败（传输失败、HTTP 非 2xx、Lambda 函数内部错误）都通过 err 表达，不返回多余信息。
//
// 主入口是包级 Invoke；需要注入配置（AWS config、mock、超时、本地重定向）时
// 用 New(opts...) 构造 Client 再调 Client.Invoke。
package awsease

import (
	"context"
	"sync"
)

// Version 是当前模块语义化版本号。
const Version = "0.4.2"

// backend 是后端类型，三选一，由 url 的 scheme 推导。
type backend string

const (
	backendHTTP   backend = "http"   // http:// 或 https://，标准 HTTP 请求/响应
	backendLambda backend = "lambda" // lambda://<fn>，lambda.Invoke
	backendSQS    backend = "sqs"    // sqs://<queue>，sqs.SendMessage
)

var (
	defaultClient     *Client
	defaultClientOnce sync.Once
)

// Invoke 用包级默认客户端执行一次调用（默认超时、默认 AWS 凭证链，惰性初始化、并发安全）。
// 需要细控基础设施时用 New(opts...).Invoke。
//
// url 形如 backend://target[?特性参数]：
//
//	http/https：整串即请求 URL（含 query，原样透传）；方法恒为 POST，无特性参数。
//	lambda://<fn>[?async=true|1]：fn 是函数名或 ARN；async=true（或 1）即发即忘（返回 body 为 nil）。
//	sqs://<queue|queue-url>[?group=g&dedup=d&attr.k=v]：group/dedup 是 FIFO 字段，
//	    attr.<key>=<value> 映射为 String 类型 MessageAttributes（可多个）。发送成功返回 body 为 nil。
//
// 错误约定：err 非 nil 即本次调用失败（地址非法、传输失败、HTTP 非 2xx、Lambda 函数内部错误），
// 此时 body 恒为 nil；err 为 nil 时 body 是后端返回的有效数据（HTTP 响应体 / Lambda 返回 payload）。
func Invoke(ctx context.Context, url string, payload []byte) ([]byte, error) {
	defaultClientOnce.Do(func() { defaultClient = New() })
	return defaultClient.Invoke(ctx, url, payload)
}
