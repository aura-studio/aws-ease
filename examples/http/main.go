// Command http 演示用 aws-ease 调普通 HTTP 后端。
//
// 统一调用模式：c.Do(ctx, "http://"+目标, []byte(payload))。
// Body 非空 -> 默认 POST、默认 header；path/query 直接写在目标里。
// 需要 https 时把完整 https:// 地址传给 Do 即可（Do 接受任意 backend://target）。
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

	c := awsease.New()

	resp, err := c.Do(context.Background(), "http://"+target, []byte(`{"event":"created","id":1}`))
	if err != nil {
		log.Fatalf("transport error: %v", err)
	}
	fmt.Printf("backend=%s ok=%v status=%d funcError=%q messageID=%q body=%s\n",
		resp.Backend, resp.OK(), resp.Status, resp.FuncError, resp.MessageID, resp.String())
}
