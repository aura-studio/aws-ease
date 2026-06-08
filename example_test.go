package awsease

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
)

func ExampleClient_Call() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	client := New(WithRewrite("lambda://*", server.URL+"/{host}{path}"))
	resp, err := client.Call(context.Background(), "lambda://svc/do", []byte("payload"))
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(resp.StatusCode, string(resp.Body))
	// Output: 200 ok
}
