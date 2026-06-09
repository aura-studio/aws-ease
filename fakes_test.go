package awsease

import (
	"context"

	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
)

// fakeLambda 是 LambdaAPI 的内存桩，记录入参、回放预设出参。
type fakeLambda struct {
	in  *awslambda.InvokeInput
	out *awslambda.InvokeOutput
	err error
}

func (f *fakeLambda) Invoke(_ context.Context, in *awslambda.InvokeInput, _ ...func(*awslambda.Options)) (*awslambda.InvokeOutput, error) {
	f.in = in
	return f.out, f.err
}

// fakeSQS 是 SQSAPI 的内存桩，记录入参、回放预设出参，并统计 GetQueueUrl 调用次数（验证缓存）。
type fakeSQS struct {
	sendIn  *awssqs.SendMessageInput
	sendOut *awssqs.SendMessageOutput
	sendErr error

	queueIn    *awssqs.GetQueueUrlInput
	queueOut   *awssqs.GetQueueUrlOutput
	queueErr   error
	queueCalls int
}

func (f *fakeSQS) SendMessage(_ context.Context, in *awssqs.SendMessageInput, _ ...func(*awssqs.Options)) (*awssqs.SendMessageOutput, error) {
	f.sendIn = in
	return f.sendOut, f.sendErr
}

func (f *fakeSQS) GetQueueUrl(_ context.Context, in *awssqs.GetQueueUrlInput, _ ...func(*awssqs.Options)) (*awssqs.GetQueueUrlOutput, error) {
	f.queueIn = in
	f.queueCalls++
	return f.queueOut, f.queueErr
}
