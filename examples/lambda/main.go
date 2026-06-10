// Command lambda 演示用 aws-ease 调 AWS Lambda。
//
// 统一调用模式：c.Invoke(ctx, "lambda://<fn>", payload) -> (body, err)。
// payload 原样即 lambda payload（无信封），body 即函数返回的 payload 字节；
// 函数内部报错（FunctionError）也走 err，错误串含函数错误名与错误 payload；
// 异步即发即忘在 URL 上加 ?async=true（或 ?async=1），成功返回 nil body。
//
// 运行（需 AWS 凭证）：
//
//	AWS_REGION=us-east-1 AWS_EASE_LAMBDA_TARGET=my-func go run ./examples/lambda
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
	target := os.Getenv("AWS_EASE_LAMBDA_TARGET")
	if target == "" {
		fmt.Println("set AWS_EASE_LAMBDA_TARGET=<function-name> (and AWS creds) to run this demo")
		return
	}

	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Fatalf("load aws config: %v", err)
	}
	c := awsease.New(awsease.WithAWSConfig(cfg))

	// err 非 nil 即失败（地址非法、传输失败、函数内部错误都在这里）；成功时 body 即返回 payload。
	body, err := c.Invoke(context.Background(), "lambda://"+target, []byte(`{"event":"created","id":1}`))
	if err != nil {
		log.Fatalf("invoke failed: %v", err)
	}
	fmt.Printf("payload=%s\n", body)
}
