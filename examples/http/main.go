// Command http 演示用 aws-ease 调普通 HTTP 后端。
//
// 统一调用模式：awsease.Invoke(ctx, url, payload) -> (body, err)。
// payload 非空 -> 默认 POST；path/query 直接写在 URL 里；
// 需要自定义方法/请求头时用保留字参数 ?ease.method=DELETE&ease.header.X-Custom=v
// （以 "ease." 开头的参数发出前会被剥除，其余 query 原样保留）。
//
// 运行（需一个可 POST 的端点，如 httpbin.org/post）：
//
//	AWS_EASE_HTTP_TARGET=httpbin.org/post go run ./examples/http
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	awsease "github.com/aura-studio/aws-ease"
)

func main() {
	target := os.Getenv("AWS_EASE_HTTP_TARGET")
	if target == "" {
		fmt.Println("set AWS_EASE_HTTP_TARGET=<host[/path]> to run this demo")
		return
	}

	// err 非 nil 即失败（传输失败或 HTTP 非 2xx，错误串含 status 与响应体）；
	// 成功时 body 就是响应体字节，无需再判状态码。
	body, err := awsease.Invoke(context.Background(), "http://"+target, []byte(`{"event":"created","id":1}`))
	if err != nil {
		log.Fatalf("invoke failed: %v", err)
	}
	fmt.Printf("payload=%s\n", body)
}
