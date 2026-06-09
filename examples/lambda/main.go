// Command lambda 演示用 aws-ease 调 AWS Lambda（同步 Invoke + 异步 Event）。
//
// 运行（需 AWS 凭证）：
//
//	AWS_REGION=us-east-1 AWS_EASE_LAMBDA_FN=my-func go run ./examples/lambda
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-sdk-go-v2/config"

	awsease "github.com/aura-studio/aws-ease"
)

func main() {
	fn := os.Getenv("AWS_EASE_LAMBDA_FN")
	if fn == "" {
		fmt.Println("set AWS_EASE_LAMBDA_FN=<function-name> (and AWS creds) to run this demo")
		return
	}

	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Fatalf("load aws config: %v", err)
	}
	c := awsease.New(awsease.WithAWSConfig(cfg))
	ctx := context.Background()

	// 1) 同步 Invoke：Body 原样即 payload（无信封），返回 payload 在 resp.Body。
	resp, err := c.Do(ctx, "lambda://"+fn, []byte(`{"hello":"aws-ease"}`))
	if err != nil {
		log.Fatalf("transport error: %v", err) // 鉴权 / 网络 / 函数不存在
	}
	if resp.FuncError != "" { // 函数内部抛错，诚实暴露（不是假 502，也不是传输 error）
		log.Fatalf("function error %s: %s", resp.FuncError, resp)
	}
	fmt.Printf("sync invoke ok, returned payload: %s\n", resp.String())

	// 2) 异步 Event（即发即忘）：resp.OK() 仅表示已被 AWS 接受投递，不代表函数已执行成功。
	resp, err = c.DoRequest(ctx, awsease.Request{
		Target: "lambda://" + fn,
		Body:   []byte(`{"async":true}`),
		Async:  true,
	})
	if err != nil {
		log.Fatalf("transport error: %v", err)
	}
	fmt.Printf("async invoke accepted=%v (body is nil: %v)\n", resp.OK(), resp.Body == nil)
}
