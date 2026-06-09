// Command aws-ease-mock 是一个最小的本地 HTTP mock，用于配合 awsease.WithLocalRedirect 联调。
//
// 它暴露两个路由，把被重定向过来的 lambda:// / sqs:// 调用裸回显：
//
//	POST {base}/lambda/<fn>     -> 200，响应体 = 请求体（payload 原样回显）
//	POST {base}/sqs/<queue>     -> 200，响应体 = 请求体（消息体原样回显）
//
// 方法与路径放在响应头里（X-AWS-Ease-Method / X-AWS-Ease-Path / X-AWS-Ease-Mock），
// 不污染响应体——与新设计「Body 即 payload，所见即所得」一致，没有任何信封字段。
package main

import (
	"io"
	"log"
	"net/http"
	"os"
)

func main() {
	addr := os.Getenv("AWS_EASE_MOCK_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	log.Printf("aws-ease mock listening on %s", addr)
	if err := http.ListenAndServe(addr, newMux()); err != nil {
		log.Fatal(err)
	}
}

// newMux 构造 mock 的路由表（抽出便于测试）。
func newMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/lambda/", echo("lambda"))
	mux.HandleFunc("/sqs/", echo("sqs"))
	return mux
}

// echo 裸回显请求体；方法/路径/后端名放进响应头，不进响应体。
func echo(backend string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("X-AWS-Ease-Mock", backend)
		w.Header().Set("X-AWS-Ease-Method", r.Method)
		w.Header().Set("X-AWS-Ease-Path", r.URL.Path)
		if ct := r.Header.Get("Content-Type"); ct != "" {
			w.Header().Set("Content-Type", ct)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}
}
