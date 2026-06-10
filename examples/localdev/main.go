// Command localdev 演示本地开发：用 WithLocalRedirect 把 lambda:// 与 sqs:// 打到一个
// 进程内的 HTTP mock，业务地址串一字不改、完全不碰真实 AWS。
// 重定向规则：lambda://<fn> -> {base}/lambda/<fn>，sqs://<q> -> {base}/sqs/<q>；
// 特性参数（async/group/attr.k 等）原样转为重定向 URL 的 query，会出现在 mock 收到的请求里。
// 返回值语义与生产一致：lambda 同步调用拿到 mock 的响应体；sqs 与 lambda?async=true 是
// 推送/即发即忘语义，成功一律返回 nil body——本地验证过的行为切回真实 AWS 不变。
//
// 运行（无需任何凭证）：
//
//	go run ./examples/localdev
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"

	awsease "github.com/aura-studio/aws-ease"
)

func main() {
	// 一个最小 mock：回显请求体（约定同 cmd/aws-ease-mock）。
	// 打印收到的 path?query：特性参数（如下面 sqs 的 attr.type）会原样出现在 query 里，便于观察断言。
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Printf("mock received: %s %s\n", r.Method, r.URL.RequestURI())
		body, _ := io.ReadAll(r.Body)
		_, _ = w.Write(body)
	}))
	defer mock.Close()

	// 关键一行：把 lambda://、sqs:// 重定向到本地 mock。业务代码无需感知。
	c := awsease.New(awsease.WithLocalRedirect(mock.URL))
	ctx := context.Background()

	// lambda://（同步）：与生产一致，能拿到响应体——本地即 mock 回显的请求体。
	body, err := c.Invoke(ctx, "lambda://order-create", []byte(`{"event":"created","id":1}`))
	if err != nil {
		log.Fatalf("lambda://order-create: %v", err)
	}
	fmt.Printf("lambda://order-create -> payload=%s\n", body)

	// sqs://：推送语义与生产一致，成功返回 nil body（mock 的响应体不会透出），err 为 nil 即已发送。
	if _, err := c.Invoke(ctx, "sqs://order-events?attr.type=order", []byte(`{"event":"created","id":1}`)); err != nil {
		log.Fatalf("sqs://order-events: %v", err)
	}
	fmt.Println("sqs://order-events    -> sent (nil body)")

	// 顺带演示错误处理：未知 scheme 也走 err，可用 errors.Is 哨兵判别。
	if _, err := c.Invoke(ctx, "ftp://nope", nil); errors.Is(err, awsease.ErrUnknownScheme) {
		fmt.Printf("unknown scheme handled: %v\n", err)
	}
}
