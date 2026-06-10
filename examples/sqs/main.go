// Command sqs 演示用 aws-ease 推送 SQS 消息。
//
// 统一调用模式：c.Do(ctx, "sqs://"+目标, []byte(payload))。
// 目标可以是队列名（走 GetQueueUrl 解析+缓存）或完整队列 URL；回执见 resp.MessageID；
// FIFO 的 GroupID/DedupID 与属性用 DoRequest（本例只演示统一的 Do）。
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

	resp, err := c.Do(context.Background(), "sqs://"+target, []byte(`{"event":"created","id":1}`))
	if err != nil {
		log.Fatalf("transport error: %v", err)
	}
	fmt.Printf("backend=%s ok=%v status=%d funcError=%q messageID=%q body=%s\n",
		resp.Backend, resp.OK(), resp.Status, resp.FuncError, resp.MessageID, resp.String())
}
