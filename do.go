package awsease

import (
	"context"
	"errors"
)

var errNotImplemented = errors.New("awsease: not implemented")

// Do 是主入口：解析 target 的 scheme -> 选后端 -> 执行 -> 返回统一 Response。
// 等价于 DoRequest(ctx, Request{Target: target, Body: body})。
func (c *Client) Do(ctx context.Context, target string, body []byte) (*Response, error) {
	return c.DoRequest(ctx, Request{Target: target, Body: body})
}

// DoRequest 是带细粒度控制的入口（HTTP method/header、Lambda 异步、SQS 属性/FIFO）。
//
// 错误约定：返回的 error 仅表示传输层失败，且 error 非 nil 时 resp 为 nil；
// 业务层失败（HTTP 非 2xx / Lambda FuncError 非空）不返回 error，而是 resp.OK()==false。
func (c *Client) DoRequest(ctx context.Context, req Request) (*Response, error) {
	// 由 T07 实现（parseTarget -> 可选重定向 -> switch backend -> 统一 context.WithTimeout）。
	return nil, errNotImplemented
}

// doHTTP 执行 HTTP 后端。由 T04 实现。
func (c *Client) doHTTP(ctx context.Context, req Request, url string) (*Response, error) {
	return nil, errNotImplemented
}

// doLambda 执行 Lambda 后端。由 T05 实现。
func (c *Client) doLambda(ctx context.Context, req Request, function string) (*Response, error) {
	return nil, errNotImplemented
}

// doSQS 执行 SQS 后端。由 T06 实现。
func (c *Client) doSQS(ctx context.Context, req Request, queue string) (*Response, error) {
	return nil, errNotImplemented
}
