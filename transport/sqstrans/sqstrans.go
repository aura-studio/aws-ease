package sqstrans

import (
	"context"
	"fmt"
	"time"

	"github.com/aura-studio/aws-ease"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

type SQSClient interface {
	SendMessage(ctx context.Context, params *awssqs.SendMessageInput, optFns ...func(*awssqs.Options)) (*awssqs.SendMessageOutput, error)
}

type Option func(*Transport)

// Transport 通过 SQS SendMessage 执行统一调用。
type Transport struct {
	client               SQSClient
	queueURLs            map[string]string
	messageGroupID       string
	messageDeduplication string
	baseEndpoint         string
	timeout              time.Duration
}

// New 创建 SQS transport。
func New(opts ...Option) *Transport {
	t := &Transport{
		queueURLs: map[string]string{},
		timeout:   30 * time.Second,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(t)
		}
	}
	return t
}

func WithClient(client SQSClient) Option {
	return func(t *Transport) {
		t.client = client
	}
}

func WithQueueURL(name, queueURL string) Option {
	return func(t *Transport) {
		if name == "" || queueURL == "" {
			return
		}
		t.queueURLs[name] = queueURL
	}
}

func WithFIFOFields(groupID, deduplicationID string) Option {
	return func(t *Transport) {
		t.messageGroupID = groupID
		t.messageDeduplication = deduplicationID
	}
}

func WithTimeout(timeout time.Duration) Option {
	return func(t *Transport) {
		if timeout > 0 {
			t.timeout = timeout
		}
	}
}

func WithBaseEndpoint(endpoint string) Option {
	return func(t *Transport) {
		t.baseEndpoint = endpoint
	}
}

// Invoke 将 Endpoint 转为 SQS SendMessage 请求。
func (t *Transport) Invoke(ctx context.Context, endpoint awsease.Endpoint, payload []byte) (*awsease.Response, error) {
	client, err := t.ensureClient(ctx)
	if err != nil {
		return nil, err
	}

	queueURL := endpoint.Host
	if mapped, ok := t.queueURLs[endpoint.Host]; ok {
		queueURL = mapped
	}

	input := &awssqs.SendMessageInput{
		QueueUrl:          aws.String(queueURL),
		MessageBody:       aws.String(string(payload)),
		MessageAttributes: toMessageAttributes(endpoint.Query),
	}
	if t.messageGroupID != "" {
		input.MessageGroupId = aws.String(t.messageGroupID)
	}
	if t.messageDeduplication != "" {
		input.MessageDeduplicationId = aws.String(t.messageDeduplication)
	}

	invokeCtx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()

	output, err := client.SendMessage(invokeCtx, input)
	if err != nil {
		return nil, fmt.Errorf("send sqs message to %q: %w", queueURL, err)
	}

	headers := map[string]string{}
	if output.MessageId != nil {
		headers["Message-ID"] = *output.MessageId
	}

	return &awsease.Response{StatusCode: 202, Headers: headers}, nil
}

func (t *Transport) ensureClient(ctx context.Context) (SQSClient, error) {
	if t.client != nil {
		return t.client, nil
	}

	loadOptions := []func(*config.LoadOptions) error{}
	if t.baseEndpoint != "" {
		loadOptions = append(loadOptions, config.WithBaseEndpoint(t.baseEndpoint))
	}
	cfg, err := config.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	t.client = awssqs.NewFromConfig(cfg)
	return t.client, nil
}

func toMessageAttributes(query map[string][]string) map[string]types.MessageAttributeValue {
	attrs := make(map[string]types.MessageAttributeValue, len(query))
	for key, values := range query {
		if len(values) == 0 {
			continue
		}
		attrs[key] = types.MessageAttributeValue{
			DataType:    aws.String("String"),
			StringValue: aws.String(values[0]),
		}
	}
	return attrs
}
