package awsease

import (
	"context"
	"errors"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
)

func TestDoSQSByName(t *testing.T) {
	fs := &fakeSQS{
		queueOut: &awssqs.GetQueueUrlOutput{QueueUrl: awssdk.String("https://sqs.test/123/order-events.fifo")},
		sendOut:  &awssqs.SendMessageOutput{MessageId: awssdk.String("m-1")},
	}
	c := New(WithSQSAPI(fs))

	resp, err := c.doSQS(context.Background(), Request{
		Body:       []byte(`{"event":"created"}`),
		GroupID:    "orders",
		DedupID:    "order-7",
		Attributes: map[string]string{"x-custom": "order"},
	}, "order-events.fifo")
	if err != nil {
		t.Fatalf("doSQS: %v", err)
	}

	if resp.Backend != BackendSQS || resp.Status != 0 || resp.Body != nil {
		t.Errorf("resp = %+v; want sqs/status0/nil-body", resp)
	}
	if resp.MessageID != "m-1" || !resp.OK() {
		t.Errorf("MessageID = %q, OK = %v", resp.MessageID, resp.OK())
	}
	if resp.Requested != "https://sqs.test/123/order-events.fifo" {
		t.Errorf("Requested = %q", resp.Requested)
	}
	if awssdk.ToString(fs.sendIn.QueueUrl) != "https://sqs.test/123/order-events.fifo" {
		t.Errorf("QueueUrl = %q", awssdk.ToString(fs.sendIn.QueueUrl))
	}
	if awssdk.ToString(fs.sendIn.MessageBody) != `{"event":"created"}` {
		t.Errorf("MessageBody = %q", awssdk.ToString(fs.sendIn.MessageBody))
	}
	if awssdk.ToString(fs.sendIn.MessageGroupId) != "orders" {
		t.Errorf("MessageGroupId = %q", awssdk.ToString(fs.sendIn.MessageGroupId))
	}
	if awssdk.ToString(fs.sendIn.MessageDeduplicationId) != "order-7" {
		t.Errorf("MessageDeduplicationId = %q", awssdk.ToString(fs.sendIn.MessageDeduplicationId))
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

func TestDoSQSCachesQueueURL(t *testing.T) {
	fs := &fakeSQS{
		queueOut: &awssqs.GetQueueUrlOutput{QueueUrl: awssdk.String("https://sqs.test/q")},
		sendOut:  &awssqs.SendMessageOutput{MessageId: awssdk.String("m")},
	}
	c := New(WithSQSAPI(fs))
	for i := 0; i < 3; i++ {
		if _, err := c.doSQS(context.Background(), Request{Body: []byte("x")}, "q"); err != nil {
			t.Fatalf("doSQS #%d: %v", i, err)
		}
	}
	if fs.queueCalls != 1 {
		t.Errorf("GetQueueUrl called %d times, want 1 (cache miss)", fs.queueCalls)
	}
}

func TestDoSQSQueueAsURLSkipsResolve(t *testing.T) {
	fs := &fakeSQS{sendOut: &awssqs.SendMessageOutput{MessageId: awssdk.String("m")}}
	c := New(WithSQSAPI(fs))

	resp, err := c.doSQS(context.Background(), Request{Body: []byte("x")}, "https://sqs.test/direct/q")
	if err != nil {
		t.Fatalf("doSQS: %v", err)
	}
	if fs.queueCalls != 0 {
		t.Errorf("GetQueueUrl should not be called for a URL queue, got %d", fs.queueCalls)
	}
	if awssdk.ToString(fs.sendIn.QueueUrl) != "https://sqs.test/direct/q" || resp.Requested != "https://sqs.test/direct/q" {
		t.Errorf("QueueUrl/Requested wrong: %q / %q", awssdk.ToString(fs.sendIn.QueueUrl), resp.Requested)
	}
}

func TestDoSQSNonUTF8Body(t *testing.T) {
	fs := &fakeSQS{sendOut: &awssqs.SendMessageOutput{MessageId: awssdk.String("m")}}
	c := New(WithSQSAPI(fs))

	resp, err := c.doSQS(context.Background(), Request{Body: []byte{0xff, 0xfe, 0xfd}}, "https://sqs.test/q")
	if !errors.Is(err, ErrBadTarget) {
		t.Fatalf("err = %v, want errors.Is ErrBadTarget", err)
	}
	if resp != nil {
		t.Errorf("resp must be nil, got %+v", resp)
	}
	if fs.sendIn != nil {
		t.Error("SendMessage must not be called for invalid body")
	}
}

func TestDoSQSTransportError(t *testing.T) {
	fs := &fakeSQS{sendErr: errors.New("throttled")}
	c := New(WithSQSAPI(fs))

	resp, err := c.doSQS(context.Background(), Request{Body: []byte("x")}, "https://sqs.test/q")
	if err == nil {
		t.Fatal("expected transport error")
	}
	if resp != nil {
		t.Errorf("resp must be nil on transport error, got %+v", resp)
	}
}
