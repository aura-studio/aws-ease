package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdatypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"

	awsease "github.com/aura-studio/aws-ease"
)

// reqrespReq / reqrespRsp 复刻 lambda 框架 reqresp 信封的 JSON 形状，
// 测试侧用于断言发出的信封字节与构造回放的响应。信封内是裸业务数据——
// service 应用信封由对端引擎自行包拆，客户端侧（即本库）不感知。
type reqrespReq struct {
	Path    string `json:"path,omitempty"`
	Payload []byte `json:"payload,omitempty"`
}

type reqrespRsp struct {
	Payload []byte `json:"payload,omitempty"`
	Error   string `json:"error,omitempty"`
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestLambdaTunnelSync 验证 tunnel 模式同步调用：发出的 InvokeInput.Payload 是
// reqresp 传输信封（path 带前导 "/"，裸业务 payload base64 进 payload 字段），
// 响应拆信封后返回业务数据。
func TestLambdaTunnelSync(t *testing.T) {
	payload := []byte(`{"line":"x"}`)
	want := []byte(`{"lines":1}`)
	fl := &fakeLambda{out: &awslambda.InvokeOutput{
		Payload: mustJSON(t, reqrespRsp{Payload: want}),
	}}
	c := awsease.New(awsease.WithLambdaAPI(fl))

	body, err := c.Invoke(context.Background(), "lambda://tango/api/tango/v1/upload", payload)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	if awssdk.ToString(fl.in.FunctionName) != "tango" {
		t.Errorf("FunctionName = %q, want %q", awssdk.ToString(fl.in.FunctionName), "tango")
	}
	var sent reqrespReq
	if err := json.Unmarshal(fl.in.Payload, &sent); err != nil {
		t.Fatalf("sent payload is not a reqresp envelope: %v: %s", err, fl.in.Payload)
	}
	if sent.Path != "/api/tango/v1/upload" {
		t.Errorf("envelope path = %q, want %q", sent.Path, "/api/tango/v1/upload")
	}
	if !bytes.Equal(sent.Payload, payload) {
		t.Errorf("envelope payload = %q, want %q (raw business bytes, no extra envelope)", sent.Payload, payload)
	}
	if !bytes.Equal(body, want) {
		t.Errorf("body = %q, want %q", body, want)
	}
}

// TestLambdaTunnelArnWithPath 验证 ARN（含 ":"）后接 tunnel 路径的切分正确。
func TestLambdaTunnelArnWithPath(t *testing.T) {
	arn := "arn:aws:lambda:us-east-1:123:function:tango"
	fl := &fakeLambda{out: &awslambda.InvokeOutput{
		Payload: mustJSON(t, reqrespRsp{Payload: []byte("ok")}),
	}}
	c := awsease.New(awsease.WithLambdaAPI(fl))

	if _, err := c.Invoke(context.Background(), "lambda://"+arn+"/api/x", []byte("{}")); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if awssdk.ToString(fl.in.FunctionName) != arn {
		t.Errorf("FunctionName = %q, want %q", awssdk.ToString(fl.in.FunctionName), arn)
	}
	var sent reqrespReq
	if err := json.Unmarshal(fl.in.Payload, &sent); err != nil {
		t.Fatal(err)
	}
	if sent.Path != "/api/x" {
		t.Errorf("envelope path = %q, want %q", sent.Path, "/api/x")
	}
}

// TestLambdaTunnelInBandError 验证 in-band 错误（Response.error 非空）翻译成 err，
// body 恒为 nil，错误串含函数名与路径上下文 —— 这是 tunnel 模式相对 raw 模式的
// 关键差别：投递失败不再静默。
func TestLambdaTunnelInBandError(t *testing.T) {
	fl := &fakeLambda{out: &awslambda.InvokeOutput{
		Payload: mustJSON(t, reqrespRsp{Error: "404 page not found: /api/nope"}),
	}}
	c := awsease.New(awsease.WithLambdaAPI(fl))

	body, err := c.Invoke(context.Background(), "lambda://tango/api/nope", []byte("{}"))
	if err == nil {
		t.Fatal("in-band error must surface as err")
	}
	for _, frag := range []string{"404 page not found: /api/nope", `"tango"`, `"/api/nope"`} {
		if !strings.Contains(err.Error(), frag) {
			t.Errorf("err = %v, want it to contain %q", err, frag)
		}
	}
	if body != nil {
		t.Errorf("body = %q, want nil", body)
	}
}

// TestLambdaTunnelEmptyResponse 验证空 tunnel 响应（引擎对无返回体的成功
// 调用 marshal Response{} 即 "{}"）：成功且 body 为空。
func TestLambdaTunnelEmptyResponse(t *testing.T) {
	fl := &fakeLambda{out: &awslambda.InvokeOutput{Payload: []byte(`{}`)}}
	c := awsease.New(awsease.WithLambdaAPI(fl))

	body, err := c.Invoke(context.Background(), "lambda://tango/api/x", []byte("{}"))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if len(body) != 0 {
		t.Errorf("body = %q, want empty", body)
	}
}

// TestLambdaTunnelBinaryPayload 验证非 UTF-8 的二进制 payload 经 base64 框架
// 往返不损坏。
func TestLambdaTunnelBinaryPayload(t *testing.T) {
	payload := []byte{0x00, 0xff, 0xfe, 0x80, 0x7f}
	want := []byte{0xde, 0xad, 0xbe, 0xef}
	fl := &fakeLambda{out: &awslambda.InvokeOutput{
		Payload: mustJSON(t, reqrespRsp{Payload: want}),
	}}
	c := awsease.New(awsease.WithLambdaAPI(fl))

	body, err := c.Invoke(context.Background(), "lambda://bin/api/x", payload)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	var sent reqrespReq
	if err := json.Unmarshal(fl.in.Payload, &sent); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(sent.Payload, payload) {
		t.Errorf("binary payload corrupted: % x, want % x", sent.Payload, payload)
	}
	if !bytes.Equal(body, want) {
		t.Errorf("binary body corrupted: % x, want % x", body, want)
	}
}

// TestLambdaTunnelAsync 验证 tunnel 异步：请求仍然包信封、InvocationType=Event、
// 成功返回 (nil, nil)，不解析响应。
func TestLambdaTunnelAsync(t *testing.T) {
	fl := &fakeLambda{out: &awslambda.InvokeOutput{}}
	c := awsease.New(awsease.WithLambdaAPI(fl))

	body, err := c.Invoke(context.Background(), "lambda://tango/api/x?async=true", []byte(`{"k":1}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if body != nil {
		t.Errorf("async body = %q, want nil", body)
	}
	if fl.in.InvocationType != lambdatypes.InvocationTypeEvent {
		t.Errorf("InvocationType = %q, want Event", fl.in.InvocationType)
	}
	var sent reqrespReq
	if err := json.Unmarshal(fl.in.Payload, &sent); err != nil {
		t.Fatal(err)
	}
	if sent.Path != "/api/x" {
		t.Errorf("envelope path = %q, want %q", sent.Path, "/api/x")
	}
}

// TestLambdaTunnelFunctionErrorFirst 验证 FunctionError 优先于信封解析：
// 函数崩溃时返回的 errorMessage JSON 不会被误当 tunnel 响应解析。
func TestLambdaTunnelFunctionErrorFirst(t *testing.T) {
	fl := &fakeLambda{out: &awslambda.InvokeOutput{
		Payload:       []byte(`{"errorMessage":"boom"}`),
		FunctionError: awssdk.String("Unhandled"),
	}}
	c := awsease.New(awsease.WithLambdaAPI(fl))

	_, err := c.Invoke(context.Background(), "lambda://tango/api/x", []byte("{}"))
	if err == nil || !strings.Contains(err.Error(), "Unhandled") {
		t.Fatalf("err = %v, want FunctionError to surface first", err)
	}
}

// TestLambdaTunnelBadResponse 验证对端不是 reqresp 函数（响应不是信封 JSON）时报
// ErrBadResponse（对端问题，与地址非法的 ErrBadTarget 区分开），而不是把原始字节
// 当成功结果返回。
func TestLambdaTunnelBadResponse(t *testing.T) {
	fl := &fakeLambda{out: &awslambda.InvokeOutput{Payload: []byte("OK")}}
	c := awsease.New(awsease.WithLambdaAPI(fl))

	_, err := c.Invoke(context.Background(), "lambda://echo/api/x", []byte("{}"))
	if err == nil || !errors.Is(err, awsease.ErrBadResponse) {
		t.Fatalf("err = %v, want ErrBadResponse for a non-tunnel response", err)
	}
	if errors.Is(err, awsease.ErrBadTarget) {
		t.Errorf("err = %v, must NOT be ErrBadTarget (remote-side issue, not address)", err)
	}
}

// TestLambdaRawStillRaw 验证不带路径的 lambda url 行为不变：无信封、原样透传。
func TestLambdaRawStillRaw(t *testing.T) {
	payload := []byte(`{"line":"x"}`)
	fl := &fakeLambda{out: &awslambda.InvokeOutput{Payload: []byte("OK")}}
	c := awsease.New(awsease.WithLambdaAPI(fl))

	body, err := c.Invoke(context.Background(), "lambda://echo", payload)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !bytes.Equal(fl.in.Payload, payload) {
		t.Errorf("raw payload = %q, want %q (envelope leaked into raw mode?)", fl.in.Payload, payload)
	}
	if !bytes.Equal(body, []byte("OK")) {
		t.Errorf("body = %q, want OK", body)
	}
}

// TestLambdaTunnelBadTargets 验证非法地址快速失败【且不触达后端】：空/空段路径、
// 空函数名、未知特性参数键、非法 async 值。
func TestLambdaTunnelBadTargets(t *testing.T) {
	for _, target := range []string{
		"lambda://fn/",
		"lambda://fn//x",
		"lambda://fn/api/x/",
		"lambda:///api/x",
		"lambda://fn/api/x?envelope=service",
		"lambda://fn/api/x?foo=1",
		"lambda://fn?async=yes",
	} {
		fl := &fakeLambda{out: &awslambda.InvokeOutput{}}
		c := awsease.New(awsease.WithLambdaAPI(fl))
		if _, err := c.Invoke(context.Background(), target, []byte("{}")); !errors.Is(err, awsease.ErrBadTarget) {
			t.Errorf("Invoke(%q) err = %v, want ErrBadTarget", target, err)
		}
		if fl.in != nil {
			t.Errorf("Invoke(%q) reached the backend; parse must fail fast before any invoke", target)
		}
	}
}

// TestLambdaTunnelRedirect 验证本地重定向下的 tunnel 模式：mock 收到的请求体
// 就是真实 InvokeInput.Payload（信封字节）、URL 含函数名与路径，同步响应同样拆信封。
func TestLambdaTunnelRedirect(t *testing.T) {
	payload := []byte(`{"line":"x"}`)
	want := []byte(`{"lines":1}`)

	var gotPath string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(r.Body)
		gotBody = buf.Bytes()
		_, _ = w.Write(mustJSON(t, reqrespRsp{Payload: want}))
	}))
	defer srv.Close()

	c := awsease.New(awsease.WithLocalRedirect(srv.URL))
	body, err := c.Invoke(context.Background(), "lambda://tango/api/tango/v1/upload", payload)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if gotPath != "/lambda/tango/api/tango/v1/upload" {
		t.Errorf("redirect path = %q, want %q", gotPath, "/lambda/tango/api/tango/v1/upload")
	}
	var sent reqrespReq
	if err := json.Unmarshal(gotBody, &sent); err != nil {
		t.Fatalf("redirect body is not the envelope bytes: %v: %s", err, gotBody)
	}
	if sent.Path != "/api/tango/v1/upload" || !bytes.Equal(sent.Payload, payload) {
		t.Errorf("redirect envelope = %+v, want path=/api/tango/v1/upload payload=%q", sent, payload)
	}
	if !bytes.Equal(body, want) {
		t.Errorf("body = %q, want %q", body, want)
	}
}

// TestLambdaTunnelRedirectAsync 验证重定向 × tunnel × 异步：请求体仍是信封字节、
// 成功返回 (nil, nil)、不解析 mock 的响应体。
func TestLambdaTunnelRedirectAsync(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(r.Body)
		gotBody = buf.Bytes()
		_, _ = w.Write([]byte("ignored"))
	}))
	defer srv.Close()

	c := awsease.New(awsease.WithLocalRedirect(srv.URL))
	body, err := c.Invoke(context.Background(), "lambda://tango/api/x?async=1", []byte(`{"k":1}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if body != nil {
		t.Errorf("async body = %q, want nil", body)
	}
	var sent reqrespReq
	if err := json.Unmarshal(gotBody, &sent); err != nil || sent.Path != "/api/x" {
		t.Errorf("redirect async body = %s (err %v), want envelope with path /api/x", gotBody, err)
	}
}
