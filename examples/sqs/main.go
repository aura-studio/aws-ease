// Command sqs 演示用 aws-ease 推送 SQS 消息（普通推送 + FIFO/属性）。
//
// 运行（需 AWS 凭证）：
//
//	AWS_REGION=us-east-1 AWS_EASE_SQS_QUEUE=my-queue go run ./examples/sqs
//
// 队列可以是队列名（走 GetQueueUrl 解析+缓存）或完整队列 URL。FIFO 队列名以 .fifo 结尾。
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"

	awsease "github.com/aura-studio/aws-ease"
)

func main() {
	queue := os.Getenv("AWS_EASE_SQS_QUEUE")
	if queue == "" {
		fmt.Println("set AWS_EASE_SQS_QUEUE=<queue-name | queue-url> (and AWS creds) to run this demo")
		return
	}

	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Fatalf("load aws config: %v", err)
	}
	c := awsease.New(awsease.WithAWSConfig(cfg))
	ctx := context.Background()

	// 1) 普通推送：回执在 resp.MessageID；Status/Body 诚实为零值。
	resp, err := c.Do(ctx, "sqs://"+queue, []byte(`{"event":"created","id":1}`))
	if err != nil {
		log.Fatalf("transport error: %v", err)
	}
	fmt.Printf("sent: MessageID=%s (status=%d, body nil=%v)\n", resp.MessageID, resp.Status, resp.Body == nil)

	// 2) FIFO + 属性：用 DoRequest 细控 GroupID / DedupID / Attributes。
	if strings.HasSuffix(queue, ".fifo") {
		resp, err = c.DoRequest(ctx, awsease.Request{
			Target:     "sqs://" + queue,
			Body:       []byte(`{"event":"created","id":2}`),
			GroupID:    "orders",
			DedupID:    "order-2",
			Attributes: map[string]string{"type": "order"},
		})
		if err != nil {
			log.Fatalf("transport error: %v", err)
		}
		fmt.Printf("FIFO sent: MessageID=%s\n", resp.MessageID)
	}
}
