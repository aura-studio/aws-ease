// Command sqs 演示用 aws-ease 推送 SQS 消息。
//
// 统一调用模式：c.Invoke(ctx, "sqs://<queue>", payload) -> (body, err)。
// 目标可以是队列名（走 GetQueueUrl 解析+缓存）或完整队列 URL；
// FIFO 与属性走 URL 参数 ?group=g&dedup=d&attr.k=v（attr.<key> 映射为 String 类型 MessageAttributes）。
// 发送成功返回 nil body，不带 MessageId 等回执——能用 err 判成败就够了。
//
// 运行（需 AWS 凭证）：
//
//	AWS_REGION=us-east-1 AWS_EASE_SQS_TARGET=my-queue go run ./examples/sqs
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
	target := os.Getenv("AWS_EASE_SQS_TARGET")
	if target == "" {
		fmt.Println("set AWS_EASE_SQS_TARGET=<queue-name | queue-url> (and AWS creds) to run this demo")
		return
	}

	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Fatalf("load aws config: %v", err)
	}
	c := awsease.New(awsease.WithAWSConfig(cfg))

	// 发送类后端没有响应体：err 为 nil 即已入队（body 恒为 nil）。
	if _, err := c.Invoke(context.Background(), "sqs://"+target, []byte(`{"event":"created","id":1}`)); err != nil {
		log.Fatalf("invoke failed: %v", err)
	}
	fmt.Println("message sent")
}
