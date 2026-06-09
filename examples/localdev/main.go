// Command localdev 演示本地开发：用 WithLocalRedirect 把 lambda:// 与 sqs:// 打到一个
// 进程内的 HTTP mock，业务地址串一字不改、完全不碰真实 AWS。
//
// 运行（无需任何凭证）：
//
//	go run ./examples/localdev
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"

	awsease "github.com/aura-studio/aws-ease"
)

func main() {
	// 一个最小 mock：回显请求体，并把路径放进响应头（约定同 cmd/aws-ease-mock）。
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("X-Path", r.URL.Path)
		_, _ = w.Write(body)
	}))
	defer mock.Close()

	// 关键一行：把 lambda://、sqs:// 重定向到本地 mock。业务代码无需感知。
	c := awsease.New(awsease.WithLocalRedirect(mock.URL))
	ctx := context.Background()

	// 这两条调用在生产会真的打到 Lambda / SQS；本地全部落到 mock。
	// 注意 resp.Backend 会如实显示 http —— 因为重定向后确实走了 HTTP。
	for _, target := range []string{"lambda://order-create", "sqs://order-events"} {
		resp, err := c.Do(ctx, target, []byte(`{"hello":"aws-ease"}`))
		if err != nil {
			log.Fatalf("%s: %v", target, err)
		}
		fmt.Printf("%-22s -> backend=%s status=%d ok=%v path=%s body=%s\n",
			target, resp.Backend, resp.Status, resp.OK(), resp.Header.Get("X-Path"), resp.String())
	}

	// 顺带演示错误处理：未知 scheme 是传输层错误，可用 errors.Is 判别。
	if _, err := c.Do(ctx, "ftp://nope", nil); err != nil {
		fmt.Printf("unknown scheme handled: %v\n", err)
	}
}
