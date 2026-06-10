package tests

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"

	awsease "github.com/aura-studio/aws-ease"
)

// redirectHit 是重定向 mock 收到的一次请求快照（method / path / RawQuery / body）。
type redirectHit struct {
	method string
	path   string
	query  string
	body   string
}

// newRedirectServer 起一个记录型 mock：每收一个请求记一条 redirectHit，恒回 200 + "mock-reply"。
// 用快照切片而非布尔标记：既能断言「收到了什么」，也能断言「一个请求都没收到」（len==0）。
func newRedirectServer() (*httptest.Server, *[]redirectHit) {
	hits := &[]redirectHit{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		*hits = append(*hits, redirectHit{
			method: r.Method,
			path:   r.URL.Path,
			query:  r.URL.RawQuery,
			body:   string(b),
		})
		_, _ = w.Write([]byte("mock-reply"))
	}))
	return srv, hits
}

// TestRedirectSQSFeaturesBecomeQuery 验证 WithLocalRedirect 下 sqs:// 的特性参数原样转为
// 重定向 URL 的 query 供 mock 观察（sqs://<q>?group=g -> {base}/sqs/<q>?group=g）；
// 返回值保持生产推送语义 (nil, nil)，且真实 SQS 客户端被整体绕开（零调用）。
func TestRedirectSQSFeaturesBecomeQuery(t *testing.T) {
	srv, hits := newRedirectServer()
	defer srv.Close()
	fs := &fakeSQS{}
	c := awsease.New(awsease.WithLocalRedirect(srv.URL), awsease.WithSQSAPI(fs))

	body, err := c.Invoke(context.Background(), "sqs://order-events?group=g1&dedup=d1", []byte("evt"))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if body != nil {
		t.Errorf("body = %q, want nil (sqs push semantics must hold under redirect)", body)
	}

	if len(*hits) != 1 {
		t.Fatalf("mock got %d requests, want 1", len(*hits))
	}
	h := (*hits)[0]
	if h.path != "/sqs/order-events" {
		t.Errorf("path = %q, want /sqs/order-events", h.path)
	}
	// 特性参数经 feat.Encode() 落进 query（按键排序），故断言「都在」而非整串相等。
	if !strings.Contains(h.query, "group=g1") || !strings.Contains(h.query, "dedup=d1") {
		t.Errorf("query = %q, want it to carry group=g1 and dedup=d1", h.query)
	}
	if h.method != http.MethodPost {
		t.Errorf("method = %q, want POST (non-empty payload)", h.method)
	}
	if h.body != "evt" {
		t.Errorf("mock saw body = %q, want evt (payload must pass through verbatim)", h.body)
	}
	// 重定向必须整体绕开真实后端：解析队列、发消息都不能发生。
	if fs.queueCalls != 0 {
		t.Errorf("GetQueueUrl called %d times, want 0", fs.queueCalls)
	}
	if fs.sendIn != nil {
		t.Error("SendMessage must not be called when redirected")
	}
}

// TestRedirectSQSMethodParamNotHTTPMethod 验证重定向时特性参数【不】作为 HTTP 特性参数解读：
// sqs 的 ?method=DELETE 只进重定向 URL 的 query 供 mock 观察，实际 HTTP 方法仍按
// 「payload 非空 -> POST / 空 -> GET」推导。
func TestRedirectSQSMethodParamNotHTTPMethod(t *testing.T) {
	srv, hits := newRedirectServer()
	defer srv.Close()
	c := awsease.New(awsease.WithLocalRedirect(srv.URL), awsease.WithSQSAPI(&fakeSQS{}))

	if _, err := c.Invoke(context.Background(), "sqs://q?method=DELETE", []byte("x")); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if len(*hits) != 1 {
		t.Fatalf("mock got %d requests, want 1", len(*hits))
	}
	h := (*hits)[0]
	if h.method != http.MethodPost {
		t.Errorf("method = %q, want POST (?method=… must not rewrite the HTTP method)", h.method)
	}
	if !strings.Contains(h.query, "method=DELETE") {
		t.Errorf("query = %q, want it to carry method=DELETE for the mock to observe", h.query)
	}
}

// TestRedirectSQSNonUTF8Rejected 验证重定向路径与生产同样先做 UTF-8 校验：非法消息体在
// 发出任何 HTTP 请求之前就被拒（errors.Is ErrBadTarget），mock 一个请求都收不到——
// 本地联调跑通的行为切回真实 SQS 不会突然开始报错。
func TestRedirectSQSNonUTF8Rejected(t *testing.T) {
	srv, hits := newRedirectServer()
	defer srv.Close()
	c := awsease.New(awsease.WithLocalRedirect(srv.URL), awsease.WithSQSAPI(&fakeSQS{}))

	body, err := c.Invoke(context.Background(), "sqs://q", []byte{0xff, 0xfe, 0xfd})
	if !errors.Is(err, awsease.ErrBadTarget) {
		t.Fatalf("err = %v, want errors.Is ErrBadTarget", err)
	}
	if body != nil {
		t.Errorf("body must be nil, got %q", body)
	}
	if len(*hits) != 0 {
		t.Errorf("mock got %d requests, want 0 (validation must precede the redirect request)", len(*hits))
	}
}

// TestRedirectLambdaAsync 验证 lambda://fn?async=true 的重定向：打到 /lambda/fn 且
// async=true 进 query；即发即忘语义与生产对齐——返回 (nil, nil)、mock 的响应体被吞掉；
// lambda 客户端零调用。
func TestRedirectLambdaAsync(t *testing.T) {
	srv, hits := newRedirectServer()
	defer srv.Close()
	fl := &fakeLambda{out: &awslambda.InvokeOutput{Payload: []byte("should-not-run")}}
	c := awsease.New(awsease.WithLocalRedirect(srv.URL), awsease.WithLambdaAPI(fl))

	body, err := c.Invoke(context.Background(), "lambda://fn?async=true", []byte("x"))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if body != nil {
		t.Errorf("async body = %q, want nil (mock reply must be swallowed)", body)
	}
	if len(*hits) != 1 {
		t.Fatalf("mock got %d requests, want 1", len(*hits))
	}
	h := (*hits)[0]
	if h.path != "/lambda/fn" {
		t.Errorf("path = %q, want /lambda/fn", h.path)
	}
	if !strings.Contains(h.query, "async=true") {
		t.Errorf("query = %q, want it to carry async=true", h.query)
	}
	if fl.in != nil {
		t.Error("lambda client must not be called when redirected")
	}
}

// TestRedirectLambdaSyncBodyAndPayload 补强既有 TestLocalRedirect（已覆盖 path 路由与
// lambda 客户端零调用）：同步 lambda 重定向必须把 mock 的响应体原样返回（与 sqs/异步的
// 吞 body 形成对照），payload 也要原样到达 mock。
func TestRedirectLambdaSyncBodyAndPayload(t *testing.T) {
	srv, hits := newRedirectServer()
	defer srv.Close()
	c := awsease.New(awsease.WithLocalRedirect(srv.URL), awsease.WithLambdaAPI(&fakeLambda{}))

	body, err := c.Invoke(context.Background(), "lambda://order-create", []byte("payload-bytes"))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if string(body) != "mock-reply" {
		t.Errorf("body = %q, want mock-reply (sync lambda must return the mock body)", body)
	}
	if len(*hits) != 1 {
		t.Fatalf("mock got %d requests, want 1", len(*hits))
	}
	h := (*hits)[0]
	if h.path != "/lambda/order-create" || h.method != http.MethodPost || h.body != "payload-bytes" {
		t.Errorf("mock saw %+v, want POST /lambda/order-create with body payload-bytes", h)
	}
	if h.query != "" {
		t.Errorf("query = %q, want empty (no feature params given)", h.query)
	}
}
