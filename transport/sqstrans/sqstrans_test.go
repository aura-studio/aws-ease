package sqstrans

import (
	"context"
	"testing"

	"github.com/aura-studio/aws-ease/resolver"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
)

type fakeSQSClient struct {
	input  *awssqs.SendMessageInput
	output *awssqs.SendMessageOutput
	err    error
}

func (f *fakeSQSClient) SendMessage(_ context.Context, input *awssqs.SendMessageInput, _ ...func(*awssqs.Options)) (*awssqs.SendMessageOutput, error) {
	f.input = input
	return f.output, f.err
}

func TestSQSTransportInvokeMapsBodyAndAttributes(t *testing.T) {
	t.Parallel()

	client := &fakeSQSClient{output: &awssqs.SendMessageOutput{MessageId: strptr("msg-1")}}
	transport := New(
		WithClient(client),
		WithQueueURL("orders", "https://sqs.example/orders"),
		WithFIFOFields("group-1", "dedupe-1"),
	)
	endpoint, err := resolver.Parse("sqs://orders?source=checkout&tenant=t1")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	resp, err := transport.Invoke(context.Background(), endpoint, []byte(`{"ok":true}`))
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}
	if got := *client.input.QueueUrl; got != "https://sqs.example/orders" {
		t.Fatalf("expected queue url mapping, got %q", got)
	}
	if got := *client.input.MessageBody; got != `{"ok":true}` {
		t.Fatalf("unexpected body %q", got)
	}
	if client.input.MessageAttributes["source"].StringValue == nil || *client.input.MessageAttributes["source"].StringValue != "checkout" {
		t.Fatalf("expected source attribute, got %#v", client.input.MessageAttributes)
	}
	if client.input.MessageGroupId == nil || *client.input.MessageGroupId != "group-1" {
		t.Fatalf("expected group id, got %#v", client.input.MessageGroupId)
	}
	if resp.Headers["Message-ID"] != "msg-1" {
		t.Fatalf("expected message id header, got %#v", resp.Headers)
	}
}

func TestSQSTransportInvokeUsesHostWhenNoMapping(t *testing.T) {
	t.Parallel()

	client := &fakeSQSClient{output: &awssqs.SendMessageOutput{}}
	transport := New(WithClient(client))
	endpoint, err := resolver.Parse("sqs://queue-url")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	_, err = transport.Invoke(context.Background(), endpoint, []byte("hello"))
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}
	if got := *client.input.QueueUrl; got != "queue-url" {
		t.Fatalf("expected host as queue url, got %q", got)
	}
}

func strptr(value string) *string { return &value }
