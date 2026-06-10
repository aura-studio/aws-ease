package tests

import (
	"context"
	"errors"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"

	awsease "github.com/aura-studio/aws-ease"
)

// TestSQSByNameWithAttributesAndFIFO 验证 FIFO 字段与消息属性全部走 query 特性参数：
// group/dedup 落到 FIFO 字段，attr.<key> 映射为 String 属性且键名不被归一化。
func TestSQSByNameWithAttributesAndFIFO(t *testing.T) {
	fs := &fakeSQS{
		queueOut: &awssqs.GetQueueUrlOutput{QueueUrl: awssdk.String("https://sqs.test/123/order-events.fifo")},
		sendOut:  &awssqs.SendMessageOutput{MessageId: awssdk.String("m-1")},
	}
	c := awsease.New(awsease.WithSQSAPI(fs))

	body, err := c.Invoke(context.Background(),
		"sqs://order-events.fifo?group=orders&dedup=order-7&attr.x-custom=order",
		[]byte(`{"event":"created"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	// 推送语义：发送成功返回 (nil, nil)，MessageId 等多余信息不返回。
	if body != nil {
		t.Errorf("body = %q, want nil (push semantics)", body)
	}

	if awssdk.ToString(fs.sendIn.QueueUrl) != "https://sqs.test/123/order-events.fifo" {
		t.Errorf("QueueUrl = %q", awssdk.ToString(fs.sendIn.QueueUrl))
	}
	if awssdk.ToString(fs.sendIn.MessageBody) != `{"event":"created"}` {
		t.Errorf("MessageBody = %q", awssdk.ToString(fs.sendIn.MessageBody))
	}
	if awssdk.ToString(fs.sendIn.MessageGroupId) != "orders" || awssdk.ToString(fs.sendIn.MessageDeduplicationId) != "order-7" {
		t.Errorf("FIFO fields wrong: group=%q dedup=%q",
			awssdk.ToString(fs.sendIn.MessageGroupId), awssdk.ToString(fs.sendIn.MessageDeduplicationId))
	}
	// 属性键名不被归一化（不会变成 X-Custom）。
	attr, ok := fs.sendIn.MessageAttributes["x-custom"]
	if !ok {
		t.Fatalf("attribute key 'x-custom' missing or renamed: %v", fs.sendIn.MessageAttributes)
	}
	if awssdk.ToString(attr.DataType) != "String" || awssdk.ToString(attr.StringValue) != "order" {
		t.Errorf("attr = %+v", attr)
	}
}

// TestSQSCachesQueueURL 验证队列名 -> QueueUrl 解析结果被缓存（GetQueueUrl 只调一次）。
func TestSQSCachesQueueURL(t *testing.T) {
	fs := &fakeSQS{
		queueOut: &awssqs.GetQueueUrlOutput{QueueUrl: awssdk.String("https://sqs.test/q")},
		sendOut:  &awssqs.SendMessageOutput{MessageId: awssdk.String("m")},
	}
	c := awsease.New(awsease.WithSQSAPI(fs))
	for i := 0; i < 3; i++ {
		if _, err := c.Invoke(context.Background(), "sqs://q", []byte("x")); err != nil {
			t.Fatalf("Invoke #%d: %v", i, err)
		}
	}
	if fs.queueCalls != 1 {
		t.Errorf("GetQueueUrl called %d times, want 1 (cache miss)", fs.queueCalls)
	}
}

// TestSQSQueueAsURLSkipsResolve 验证 "http" 开头的目标被视为完整队列 URL 直用，不走解析。
func TestSQSQueueAsURLSkipsResolve(t *testing.T) {
	fs := &fakeSQS{sendOut: &awssqs.SendMessageOutput{MessageId: awssdk.String("m")}}
	c := awsease.New(awsease.WithSQSAPI(fs))

	body, err := c.Invoke(context.Background(), "sqs://https://sqs.test/direct/q", []byte("x"))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if body != nil {
		t.Errorf("body = %q, want nil", body)
	}
	if fs.queueCalls != 0 {
		t.Errorf("GetQueueUrl should not be called for a URL queue, got %d", fs.queueCalls)
	}
	if awssdk.ToString(fs.sendIn.QueueUrl) != "https://sqs.test/direct/q" {
		t.Errorf("QueueUrl = %q", awssdk.ToString(fs.sendIn.QueueUrl))
	}
}

// TestSQSNonUTF8Body 验证非 UTF-8 消息体在发送前被拒（errors.Is ErrBadTarget），不发任何请求。
func TestSQSNonUTF8Body(t *testing.T) {
	fs := &fakeSQS{sendOut: &awssqs.SendMessageOutput{MessageId: awssdk.String("m")}}
	c := awsease.New(awsease.WithSQSAPI(fs))

	body, err := c.Invoke(context.Background(), "sqs://https://sqs.test/q", []byte{0xff, 0xfe, 0xfd})
	if !errors.Is(err, awsease.ErrBadTarget) {
		t.Fatalf("err = %v, want errors.Is ErrBadTarget", err)
	}
	if body != nil {
		t.Errorf("body must be nil, got %q", body)
	}
	if fs.sendIn != nil {
		t.Error("SendMessage must not be called for invalid body")
	}
}

// TestSQSTransportError 验证传输层错误透传：err 非 nil 且 body 恒为 nil。
func TestSQSTransportError(t *testing.T) {
	fs := &fakeSQS{sendErr: errors.New("throttled")}
	c := awsease.New(awsease.WithSQSAPI(fs))

	body, err := c.Invoke(context.Background(), "sqs://https://sqs.test/q", []byte("x"))
	if err == nil {
		t.Fatal("expected transport error")
	}
	if body != nil {
		t.Errorf("body must be nil on transport error, got %q", body)
	}
}
