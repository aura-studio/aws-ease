package awsease

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdatypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
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

// doHTTP 执行 HTTP 后端：url 即完整请求地址（path/query 已在其中），Body 作请求体，
// 非 2xx 不算传输错误（err==nil，OK()==false）。
func (c *Client) doHTTP(ctx context.Context, req Request, url string) (*Response, error) {
	var body io.Reader
	if len(req.Body) > 0 {
		body = bytes.NewReader(req.Body)
	}
	method := req.httpMethod()
	httpReq, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("awsease: build http request: %w", err)
	}
	for k, v := range req.Header {
		httpReq.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("awsease: http %s %s: %w", method, url, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("awsease: read http response from %s: %w", url, err)
	}

	return &Response{
		Backend:   BackendHTTP,
		Status:    resp.StatusCode,
		Header:    resp.Header,
		Body:      respBody,
		Requested: url,
	}, nil
}

// doLambda 执行 Lambda 后端：Body 原样作 Payload（无信封）；Async 走 InvocationType=Event；
// 函数内部错误用 FuncError 表达（不伪造 Status、不返回传输 error）。
func (c *Client) doLambda(ctx context.Context, req Request, function string) (*Response, error) {
	invoker, err := c.lambdaClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("awsease: lambda client: %w", err)
	}

	invType := lambdatypes.InvocationTypeRequestResponse
	if req.Async {
		invType = lambdatypes.InvocationTypeEvent
	}

	out, err := invoker.Invoke(ctx, &awslambda.InvokeInput{
		FunctionName:   awssdk.String(function),
		InvocationType: invType,
		Payload:        req.Body, // 原样透传，无 {path,query,payload} 信封
	})
	if err != nil {
		return nil, fmt.Errorf("awsease: invoke lambda %q: %w", function, err)
	}

	resp := &Response{
		Backend:   BackendLambda,
		Async:     req.Async,
		Requested: function,
	}
	if req.Async {
		return resp, nil // 即发即忘：Body/Status 留零，OK() 由 Async 判 true
	}
	resp.Body = out.Payload
	resp.FuncError = awssdk.ToString(out.FunctionError)
	return resp, nil
}

// doSQS 执行 SQS 后端：Body 须为合法 UTF-8；队列名惰性解析为 QueueUrl 并带锁缓存；
// Attributes 映射为 String 类型 MessageAttributes（键名不被归一化）；GroupID/DedupID 用于 FIFO。
// 推送语义：Status/Body 留零，回执用 MessageID 表达。
func (c *Client) doSQS(ctx context.Context, req Request, queue string) (*Response, error) {
	if !utf8.Valid(req.Body) {
		return nil, fmt.Errorf("awsease: sqs message body is not valid UTF-8: %w", ErrBadTarget)
	}

	client, err := c.sqsClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("awsease: sqs client: %w", err)
	}

	queueURL, err := c.queueURL(ctx, client, queue)
	if err != nil {
		return nil, err
	}

	in := &awssqs.SendMessageInput{
		QueueUrl:          awssdk.String(queueURL),
		MessageBody:       awssdk.String(string(req.Body)),
		MessageAttributes: toAttributes(req.Attributes),
	}
	if req.GroupID != "" {
		in.MessageGroupId = awssdk.String(req.GroupID)
	}
	if req.DedupID != "" {
		in.MessageDeduplicationId = awssdk.String(req.DedupID)
	}

	out, err := client.SendMessage(ctx, in)
	if err != nil {
		return nil, fmt.Errorf("awsease: send sqs message to %q: %w", queueURL, err)
	}

	return &Response{
		Backend:   BackendSQS,
		MessageID: awssdk.ToString(out.MessageId),
		Requested: queueURL,
	}, nil
}

// queueURL 把队列名解析为 QueueUrl：以 "http" 开头视为已是 URL 直用；否则 GetQueueUrl 并带锁缓存。
func (c *Client) queueURL(ctx context.Context, client SQSAPI, queue string) (string, error) {
	if strings.HasPrefix(queue, "http") {
		return queue, nil
	}

	c.mu.RLock()
	cached, ok := c.queueURLs[queue]
	c.mu.RUnlock()
	if ok {
		return cached, nil
	}

	out, err := client.GetQueueUrl(ctx, &awssqs.GetQueueUrlInput{QueueName: awssdk.String(queue)})
	if err != nil {
		return "", fmt.Errorf("awsease: resolve sqs queue %q: %w", queue, err)
	}
	resolved := awssdk.ToString(out.QueueUrl)

	c.mu.Lock()
	c.queueURLs[queue] = resolved
	c.mu.Unlock()
	return resolved, nil
}

// toAttributes 把 String 属性 map 转为 SQS MessageAttributes（键名原样保留，不经 HTTP 头归一化）。
func toAttributes(m map[string]string) map[string]sqstypes.MessageAttributeValue {
	if len(m) == 0 {
		return nil
	}
	attrs := make(map[string]sqstypes.MessageAttributeValue, len(m))
	for k, v := range m {
		attrs[k] = sqstypes.MessageAttributeValue{
			DataType:    awssdk.String("String"),
			StringValue: awssdk.String(v),
		}
	}
	return attrs
}
