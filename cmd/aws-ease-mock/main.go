// Command aws-ease-mock 是一个最小的本地 HTTP mock，用于配合 awsease.WithLocalRedirect 联调。
//
// 它暴露两个路由，把被重定向过来的 lambda:// / sqs:// 调用裸回显（任意方法均接受，
// 重定向调用默认 payload 非空 POST / 空 GET，实际方法记在 X-AWS-Ease-Method 响应头）：
//
//	{base}/lambda/<fn>[?特性参数]     -> 200，响应体 = 请求体（payload 原样回显）
//	{base}/sqs/<queue>[?特性参数]     -> 200，响应体 = 请求体（消息体原样回显）
//
// handler 逻辑在 internal/mock 包，便于 tests 复用。
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/aura-studio/aws-ease/internal/mock"
)

func main() {
	addr := os.Getenv("AWS_EASE_MOCK_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	log.Printf("aws-ease mock listening on %s", addr)
	if err := http.ListenAndServe(addr, mock.Handler()); err != nil {
		log.Fatal(err)
	}
}
