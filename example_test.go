package awsease_test

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"

	"github.com/aura-studio/aws-ease"
)

// HTTP：给一个 http(s):// 地址和 Body，Do 直接发起请求并返回统一 Response。
func ExampleClient_Do() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "pong")
	}))
	defer srv.Close()

	c := awsease.New()
	resp, err := c.Do(context.Background(), srv.URL+"/ping", nil) // Body 为空 -> GET
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(resp.Status, resp.String())
	// Output: 200 pong
}

// Lambda：lambda://<函数名>，Body 原样即 payload（无隐藏信封）。
// 函数内部报错经 resp.FuncError 诚实暴露（不是假 502，也不是传输 error）。
func ExampleClient_Do_lambda() {
	c := awsease.New() // 生产：awsease.New(awsease.WithAWSConfig(cfg))
	resp, err := c.Do(context.Background(), "lambda://order-create", []byte(`{"sku":"A1","qty":2}`))
	if err != nil {
		log.Fatal(err) // 传输层失败
	}
	if resp.FuncError != "" {
		log.Fatalf("lambda %s: %s", resp.FuncError, resp) // 函数内部报错
	}
	fmt.Println(resp.String()) // Lambda 返回的 payload 原样
}

// SQS：sqs://<队列名>，推送后用 resp.MessageID 取回执；Status/Body 诚实为零值。
// FIFO 与属性用 DoRequest 细控。
func ExampleClient_DoRequest() {
	c := awsease.New()
	resp, err := c.DoRequest(context.Background(), awsease.Request{
		Target:     "sqs://order-events.fifo",
		Body:       []byte(`{"event":"created","id":7}`),
		GroupID:    "orders",
		DedupID:    "order-7",
		Attributes: map[string]string{"type": "order"},
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(resp.Backend, resp.MessageID) // 例如: sqs e5f6...
}

// 本地联调：业务地址串一字不改，构造时加一行 WithLocalRedirect，
// 把 lambda:// 与 sqs:// 打到本地 HTTP mock（cmd/aws-ease-mock）。
func Example_localRedirect() {
	c := awsease.New(awsease.WithLocalRedirect("http://localhost:8080"))
	// 实际打到 http://localhost:8080/lambda/order-create
	_, _ = c.Do(context.Background(), "lambda://order-create", []byte(`{}`))
}
