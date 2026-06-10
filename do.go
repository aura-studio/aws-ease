package awsease

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdatypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

// Invoke 解析 url 的 scheme -> 选后端 -> 执行 -> 返回后端的有效数据。
// 语义与包级 Invoke 完全一致（见其文档），只是走本 Client 的配置。
func (c *Client) Invoke(ctx context.Context, target string, payload []byte) ([]byte, error) {
	b, addr, feat, err := parseTarget(target)
	if err != nil {
		return nil, err
	}

	// 统一超时：默认套一层 context.WithTimeout；调用方若已设更短 deadline，context 自动取更早者。
	if c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	if c.redirectBase != "" && (b == backendLambda || b == backendSQS) {
		return c.doRedirect(ctx, b, addr, feat, payload)
	}

	switch b {
	case backendHTTP:
		return c.doHTTP(ctx, addr, payload)
	case backendLambda:
		return c.doLambda(ctx, addr, feat, payload)
	case backendSQS:
		return c.doSQS(ctx, addr, feat, payload)
	default:
		return nil, fmt.Errorf("awsease: %w: %q", ErrUnknownScheme, b)
	}
}

// doRedirect 在设置了 WithLocalRedirect 时，把 lambda://、sqs:// 打到本地 base：
//
//	lambda://<fn>?async=true  -> {base}/lambda/<fn>?async=true
//	sqs://<q>?group=g         -> {base}/sqs/<q>?group=g
//
// 特性参数原样转成重定向 URL 的 query 供 mock 端观察，但【不】作为 HTTP 特性参数解读
// （sqs 的 ?method=… 不会改写实际 HTTP 方法）。返回值与校验同生产对齐：sqs 仍做 UTF-8
// 校验、sqs 与 lambda 异步成功返回 (nil, nil)，保证本地联调跑出的行为切到真实后端不变。
func (c *Client) doRedirect(ctx context.Context, b backend, addr string, feat url.Values, payload []byte) ([]byte, error) {
	if b == backendSQS && !utf8.Valid(payload) {
		return nil, fmt.Errorf("awsease: sqs message body is not valid UTF-8: %w", ErrBadTarget)
	}
	u := strings.TrimRight(c.redirectBase, "/") + "/" + string(b) + "/" + addr
	if len(feat) > 0 {
		u += "?" + feat.Encode()
	}
	body, err := c.doHTTP(ctx, u, payload)
	if err != nil {
		return nil, err
	}
	if b == backendSQS || isAsync(feat) {
		return nil, nil // 推送 / 即发即忘语义：与生产一致，不返回 mock 的响应体
	}
	return body, nil
}

// doHTTP 执行 HTTP 后端：addr 即完整请求地址（含 query，原样透传），payload 作请求体，
// 方法恒为 POST。非 2xx 一律视为失败，状态码与响应体放进 err。
func (c *Client) doHTTP(ctx context.Context, addr string, payload []byte) ([]byte, error) {
	var body io.Reader
	if len(payload) > 0 {
		body = strings.NewReader(string(payload))
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, addr, body)
	if err != nil {
		return nil, fmt.Errorf("awsease: build http request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("awsease: http POST %s: %w", addr, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("awsease: read http response from %s: %w", addr, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("awsease: http POST %s: status %d: %s", addr, resp.StatusCode, respBody)
	}
	return respBody, nil
}

// doLambda 执行 Lambda 后端：payload 原样透传（无信封）。
// 特性参数：async=true|1 走 InvocationType=Event 即发即忘（成功返回 nil body）。
// 函数内部错误（FunctionError 非空）视为失败，错误名与错误 payload 放进 err。
func (c *Client) doLambda(ctx context.Context, function string, feat url.Values, payload []byte) ([]byte, error) {
	invoker, err := c.lambdaClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("awsease: lambda client: %w", err)
	}

	async := isAsync(feat)
	invType := lambdatypes.InvocationTypeRequestResponse
	if async {
		invType = lambdatypes.InvocationTypeEvent
	}

	out, err := invoker.Invoke(ctx, &awslambda.InvokeInput{
		FunctionName:   awssdk.String(function),
		InvocationType: invType,
		Payload:        payload, // 原样透传，无 {path,query,payload} 信封
	})
	if err != nil {
		return nil, fmt.Errorf("awsease: invoke lambda %q: %w", function, err)
	}

	if async {
		return nil, nil // 即发即忘：已被 AWS 接受投递，不代表函数已成功执行
	}
	if funcErr := awssdk.ToString(out.FunctionError); funcErr != "" {
		return nil, fmt.Errorf("awsease: lambda %q failed: %s: %s", function, funcErr, out.Payload)
	}
	return out.Payload, nil
}

// doSQS 执行 SQS 后端：payload 须为合法 UTF-8；队列名惰性解析为 QueueUrl 并带锁缓存。
// 特性参数：group/dedup（FIFO）、attr.<key>（String 类型 MessageAttributes，键名不被归一化）。
// 推送语义：发送成功返回 nil body（回执等多余信息不返回）。
func (c *Client) doSQS(ctx context.Context, queue string, feat url.Values, payload []byte) ([]byte, error) {
	if !utf8.Valid(payload) {
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
		MessageBody:       awssdk.String(string(payload)),
		MessageAttributes: toAttributes(feat),
	}
	if g := feat.Get("group"); g != "" {
		in.MessageGroupId = awssdk.String(g)
	}
	if d := feat.Get("dedup"); d != "" {
		in.MessageDeduplicationId = awssdk.String(d)
	}

	if _, err := client.SendMessage(ctx, in); err != nil {
		return nil, fmt.Errorf("awsease: send sqs message to %q: %w", queueURL, err)
	}
	return nil, nil
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

// isAsync 报告特性参数是否要求 Lambda 即发即忘（async=true 或 async=1）。
func isAsync(feat url.Values) bool {
	v := feat.Get("async")
	return v == "true" || v == "1"
}

// toAttributes 把特性参数里 "attr." 开头的键转为 SQS MessageAttributes
// （键名去前缀后原样保留，不经 HTTP 头归一化；多值取首个）。
func toAttributes(feat url.Values) map[string]sqstypes.MessageAttributeValue {
	var attrs map[string]sqstypes.MessageAttributeValue
	for k, vs := range feat {
		name, ok := strings.CutPrefix(k, "attr.")
		if !ok || len(vs) == 0 {
			continue
		}
		if attrs == nil {
			attrs = map[string]sqstypes.MessageAttributeValue{}
		}
		attrs[name] = sqstypes.MessageAttributeValue{
			DataType:    awssdk.String("String"),
			StringValue: awssdk.String(vs[0]),
		}
	}
	return attrs
}
