// Command lambda 演示用 aws-ease 调 AWS Lambda。
//
// 统一调用模式：c.Do(ctx, "lambda://"+目标, []byte(payload))。
// Body 原样即 payload（无信封）；函数内部报错见 resp.FuncError；
// 异步等细控用 DoRequest（本例只演示统一的 Do）。
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

	resp, err := c.Do(context.Background(), "lambda://"+target, []byte(`{"event":"created","id":1}`))
	if err != nil {
		log.Fatalf("transport error: %v", err)
	}
	fmt.Printf("backend=%s ok=%v status=%d funcError=%q messageID=%q body=%s\n",
		resp.Backend, resp.OK(), resp.Status, resp.FuncError, resp.MessageID, resp.String())
}
