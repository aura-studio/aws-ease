// Command http 演示用 aws-ease 调普通 HTTP 后端。
//
// 运行（默认对 checkip.amazonaws.com 发 GET，无需凭证）：
//
//	go run ./examples/http
//
// 想跑 POST 演示，设一个可 POST 的端点：
//
//	AWS_EASE_HTTP_URL=https://your.endpoint go run ./examples/http
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	awsease "github.com/aura-studio/aws-ease"
)

func main() {
	c := awsease.New()
	ctx := context.Background()

	// 1) 最简：Body 为空 -> GET。path/query 直接写在地址里。
	resp, err := c.Do(ctx, "https://checkip.amazonaws.com/", nil)
	if err != nil {
		log.Fatalf("transport error: %v", err) // 传输层失败：网络 / 超时 / 地址非法
	}
	if !resp.OK() { // 业务层失败：HTTP 非 2xx（err 仍为 nil）
		log.Fatalf("http %d: %s", resp.Status, resp)
	}
	fmt.Printf("GET checkip -> %d, your public IP = %s", resp.Status, resp.String())

	// 2) 细控：自定义 method / header（用 DoRequest）。
	target := os.Getenv("AWS_EASE_HTTP_URL")
	if target == "" {
		fmt.Println("set AWS_EASE_HTTP_URL=https://your.endpoint to run the POST demo")
		return
	}
	resp, err = c.DoRequest(ctx, awsease.Request{
		Target: target,
		Method: http.MethodPost,
		Header: map[string]string{"Content-Type": "application/json", "X-Demo": "aws-ease"},
		Body:   []byte(`{"hello":"aws-ease"}`),
	})
	if err != nil {
		log.Fatalf("transport error: %v", err)
	}
	fmt.Printf("POST %s -> %d\n%s\n", target, resp.Status, resp.String())
}
