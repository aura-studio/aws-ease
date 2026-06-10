package tests

import (
	"context"
	"errors"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"

	awsease "github.com/aura-studio/aws-ease"
)

func TestSQSByNameWithAttributesAndFIFO(t *testing.T) {
	fs := &fakeSQS{
		queueOut: &awssqs.GetQueueUrlOutput{QueueUrl: awssdk.String("https://sqs.test/123/order-events.fifo")},
		sendOut:  &awssqs.SendMessageOutput{MessageId: awssdk.String("m-1")},
	}
	c := awsease.New(awsease.WithSQSAPI(fs))

	resp, err := c.DoRequest(context.Background(), awsease.Request{
		Target:     "sqs://order-events.fifo",
		Body:       []byte(`{"event":"created"}`),
		GroupID:    "orders",
		DedupID:    "order-7",
		Attributes: map[string]string{"x-custom": "order"},
	})
	if err != nil {
		t.Fatalf("DoRequest: %v", err)
	}

	if resp.Backend != awsease.BackendSQS || resp.Status != 0 || resp.Body != nil {
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

func TestSQSCachesQueueURL(t *testing.T) {
	fs := &fakeSQS{
		queueOut: &awssqs.GetQueueUrlOutput{QueueUrl: awssdk.String("https://sqs.test/q")},
		sendOut:  &awssqs.SendMessageOutput{MessageId: awssdk.String("m")},
	}
	c := awsease.New(awsease.WithSQSAPI(fs))
	for i := 0; i < 3; i++ {
		if _, err := c.Do(context.Background(), "sqs://q", []byte("x")); err != nil {
			t.Fatalf("Do #%d: %v", i, err)
		}
	}
	if fs.queueCalls != 1 {
		t.Errorf("GetQueueUrl called %d times, want 1 (cache miss)", fs.queueCalls)
	}
}

func TestSQSQueueAsURLSkipsResolve(t *testing.T) {
	fs := &fakeSQS{sendOut: &awssqs.SendMessageOutput{MessageId: awssdk.String("m")}}
	c := awsease.New(awsease.WithSQSAPI(fs))

	resp, err := c.Do(context.Background(), "sqs://https://sqs.test/direct/q", []byte("x"))
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if fs.queueCalls != 0 {
		t.Errorf("GetQueueUrl should not be called for a URL queue, got %d", fs.queueCalls)
	}
	if awssdk.ToString(fs.sendIn.QueueUrl) != "https://sqs.test/direct/q" || resp.Requested != "https://sqs.test/direct/q" {
		t.Errorf("QueueUrl/Requested wrong: %q / %q", awssdk.ToString(fs.sendIn.QueueUrl), resp.Requested)
	}
}

func TestSQSNonUTF8Body(t *testing.T) {
	fs := &fakeSQS{sendOut: &awssqs.SendMessageOutput{MessageId: awssdk.String("m")}}
	c := awsease.New(awsease.WithSQSAPI(fs))

	resp, err := c.Do(context.Background(), "sqs://https://sqs.test/q", []byte{0xff, 0xfe, 0xfd})
	if !errors.Is(err, awsease.ErrBadTarget) {
		t.Fatalf("err = %v, want errors.Is ErrBadTarget", err)
	}
	if resp != nil {
		t.Errorf("resp must be nil, got %+v", resp)
	}
	if fs.sendIn != nil {
		t.Error("SendMessage must not be called for invalid body")
	}
}

func TestSQSTransportError(t *testing.T) {
	fs := &fakeSQS{sendErr: errors.New("throttled")}
	c := awsease.New(awsease.WithSQSAPI(fs))

	resp, err := c.Do(context.Background(), "sqs://https://sqs.test/q", []byte("x"))
	if err == nil {
		t.Fatal("expected transport error")
	}
	if resp != nil {
		t.Errorf("resp must be nil on transport error, got %+v", resp)
	}
}
