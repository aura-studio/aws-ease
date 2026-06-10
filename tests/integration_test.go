//go:build integration

// 真实 AWS 集成测试（默认 go test 不会编译/运行，需 -tags integration）。
// 运行前需具备可用的 AWS 凭证与 region（环境变量或 ~/.aws）。
//
//	go test -tags integration -run Integration -v ./tests/
//
// TestIntegrationLambdaInvoke 会真实创建并删除一个一次性 Lambda（复用账号里现有函数的
// 执行角色，不新建 IAM），默认跳过；要跑它额外设 AWS_EASE_IT_LAMBDA_CREATE=1。
package tests

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdatypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/aws/aws-sdk-go-v2/service/sqs"

	awsease "github.com/aura-studio/aws-ease"
)

func itConfig(t *testing.T) awssdk.Config {
	t.Helper()
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-1"
	}
	cfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion(region))
	if err != nil {
		t.Fatalf("load aws config: %v", err)
	}
	return cfg
}

// TestIntegrationHTTP：对真实公网端点发 HTTP 请求，验证 HTTP 整链路。
// checkip.amazonaws.com 是 AWS 的稳定服务，GET 返回 200 + 纯文本公网 IP；
// 新 API 下 2xx 即 err 为 nil，返回体非空即视为通过。
func TestIntegrationHTTP(t *testing.T) {
	c := awsease.New()
	body, err := c.Invoke(context.Background(), "https://checkip.amazonaws.com/", nil)
	if err != nil {
		t.Fatalf("http Invoke: %v", err)
	}
	if len(body) == 0 {
		t.Errorf("expected a non-empty body (the public IP)")
	}
	t.Logf("HTTP ok, body=%q", body)
}

// TestIntegrationSQS：建临时队列 -> 用 aws-ease 按队列名发送（触发 GetQueueUrl 解析+缓存）
// -> 收回验证内容 -> 再发一次验证缓存命中 -> 清理删除队列。
func TestIntegrationSQS(t *testing.T) {
	ctx := context.Background()
	cfg := itConfig(t)
	raw := sqs.NewFromConfig(cfg)

	qname := "aws-ease-it-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	created, err := raw.CreateQueue(ctx, &sqs.CreateQueueInput{QueueName: awssdk.String(qname)})
	if err != nil {
		t.Fatalf("create queue: %v", err)
	}
	t.Cleanup(func() {
		if _, derr := raw.DeleteQueue(context.Background(), &sqs.DeleteQueueInput{QueueUrl: created.QueueUrl}); derr != nil {
			t.Logf("WARNING: failed to delete queue %s: %v", qname, derr)
			return
		}
		t.Logf("deleted queue %s", qname)
	})
	t.Logf("created queue %s -> %s", qname, awssdk.ToString(created.QueueUrl))

	c := awsease.New(awsease.WithAWSConfig(cfg))

	msg := `{"hello":"aws-ease","n":1}`
	body, err := c.Invoke(ctx, "sqs://"+qname, []byte(msg))
	if err != nil {
		t.Fatalf("sqs Invoke: %v", err)
	}
	// 推送语义：发送成功返回 (nil, nil)，MessageId 等多余信息不返回。
	if body != nil {
		t.Errorf("dishonest sqs body: %q (want nil)", body)
	}
	t.Logf("sent message to %s via aws-ease ✓", qname)

	// 收回来核对内容确实进了队列。
	var got string
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) && got == "" {
		out, rerr := raw.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:            created.QueueUrl,
			MaxNumberOfMessages: 1,
			WaitTimeSeconds:     5,
		})
		if rerr != nil {
			t.Fatalf("receive: %v", rerr)
		}
		if len(out.Messages) > 0 {
			got = awssdk.ToString(out.Messages[0].Body)
		}
	}
	if got != msg {
		t.Fatalf("received %q, want %q", got, msg)
	}
	t.Log("received message body matches sent payload ✓")

	// 第二次发送：队列名->URL 缓存命中后仍可用。
	if _, err := c.Invoke(ctx, "sqs://"+qname, []byte(`{"n":2}`)); err != nil {
		t.Fatalf("second send (cache hit): %v", err)
	}
	t.Log("second send (cached queue url) ok ✓")
}

// TestIntegrationLambdaError：调一个不存在的函数，验证真实鉴权+路由+错误传播
// （无需部署函数、无副作用），并列出账号里现有的函数。
func TestIntegrationLambdaError(t *testing.T) {
	ctx := context.Background()
	cfg := itConfig(t)
	c := awsease.New(awsease.WithAWSConfig(cfg))

	body, err := c.Invoke(ctx, "lambda://aws-ease-it-nonexistent-fn", []byte("{}"))
	if err == nil {
		t.Fatalf("expected error invoking nonexistent function, got body=%q", body)
	}
	if body != nil {
		t.Errorf("body must be nil on error, got %q", body)
	}
	t.Logf("lambda not-found surfaced as err ✓: %v", err)

	lc := lambda.NewFromConfig(cfg)
	out, lerr := lc.ListFunctions(ctx, &lambda.ListFunctionsInput{})
	if lerr != nil {
		t.Logf("ListFunctions failed: %v", lerr)
		return
	}
	t.Logf("account has %d lambda function(s):", len(out.Functions))
	for _, f := range out.Functions {
		t.Logf("  - %s", awssdk.ToString(f.FunctionName))
	}
}

// TestIntegrationLambdaInvoke：真实成功调用。建一个一次性 echo 函数（复用现有函数的执行角色，
// 不新建 IAM）-> 用 aws-ease invoke -> 断言返回的 payload 字节 -> 删除。
// 默认跳过，需 AWS_EASE_IT_LAMBDA_CREATE=1。
func TestIntegrationLambdaInvoke(t *testing.T) {
	if os.Getenv("AWS_EASE_IT_LAMBDA_CREATE") == "" {
		t.Skip("set AWS_EASE_IT_LAMBDA_CREATE=1 to create a throwaway echo Lambda and invoke it")
	}
	ctx := context.Background()
	cfg := itConfig(t)
	lc := lambda.NewFromConfig(cfg)

	// 复用账号里现有函数的执行角色，避免新建 IAM。
	list, err := lc.ListFunctions(ctx, &lambda.ListFunctionsInput{})
	if err != nil || len(list.Functions) == 0 {
		t.Skipf("no existing function to borrow an execution role from (err=%v)", err)
	}
	role := awssdk.ToString(list.Functions[0].Role)
	fnName := "aws-ease-it-echo-" + strconv.FormatInt(time.Now().UnixNano(), 10)

	if _, err := lc.CreateFunction(ctx, &lambda.CreateFunctionInput{
		FunctionName: awssdk.String(fnName),
		Runtime:      lambdatypes.Runtime("python3.13"), // 字符串字面量：老版 SDK 无 RuntimePython313 常量，API 侧照常接受
		Role:         awssdk.String(role),
		Handler:      awssdk.String("index.handler"),
		Code:         &lambdatypes.FunctionCode{ZipFile: echoZip(t)},
		Description:  awssdk.String("aws-ease integration test throwaway echo (safe to delete)"),
	}); err != nil {
		t.Fatalf("create function: %v", err)
	}
	t.Cleanup(func() {
		if _, derr := lc.DeleteFunction(context.Background(), &lambda.DeleteFunctionInput{FunctionName: awssdk.String(fnName)}); derr != nil {
			t.Logf("WARNING: failed to delete function %s: %v", fnName, derr)
			return
		}
		t.Logf("deleted function %s", fnName)
	})

	if err := waitLambdaActive(ctx, lc, fnName); err != nil {
		t.Fatalf("wait active: %v", err)
	}
	t.Logf("created+active function %s (borrowed role %s)", fnName, role)

	c := awsease.New(awsease.WithAWSConfig(cfg))
	body, err := c.Invoke(ctx, "lambda://"+fnName, []byte(`{"hello":"aws-ease"}`))
	if err != nil {
		t.Fatalf("invoke via aws-ease: %v", err)
	}
	// 同步成功即 err 为 nil，body 是函数返回的 payload 字节（无信封），直接解码断言。
	var got struct {
		Echo map[string]any `json:"echo"`
		OK   bool           `json:"ok"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode lambda payload %q: %v", body, err)
	}
	if !got.OK || got.Echo["hello"] != "aws-ease" {
		t.Fatalf("echo mismatch: %s", body)
	}
	t.Logf("lambda real invoke ok ✓, returned payload: %s", body)
}

func waitLambdaActive(ctx context.Context, lc *lambda.Client, fn string) error {
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		out, err := lc.GetFunctionConfiguration(ctx, &lambda.GetFunctionConfigurationInput{FunctionName: awssdk.String(fn)})
		if err != nil {
			return err
		}
		if out.State == lambdatypes.StateActive && out.LastUpdateStatus == lambdatypes.LastUpdateStatusSuccessful {
			return nil
		}
		if out.State == lambdatypes.StateFailed {
			return fmt.Errorf("function entered Failed state: %s", awssdk.ToString(out.StateReason))
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timed out waiting for function %s to become active", fn)
}

// echoZip 构造一个最小 python Lambda 部署包，handler 原样回显 event。
func echoZip(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create("index.py")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("def handler(event, context):\n    return {\"echo\": event, \"ok\": True}\n")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
