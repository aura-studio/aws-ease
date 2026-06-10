// Package mock 提供 aws-ease 本地联调用的 HTTP mock handler，配合 awsease.WithLocalRedirect 使用。
// cmd/aws-ease-mock 是它的命令行封装；tests 包直接复用 Handler 做单测。
package mock

import (
	"io"
	"net/http"
)

// Handler 返回 mock 的 HTTP handler：/lambda/ 与 /sqs/ 两个路由裸回显请求体，
// 方法/路径/后端名放进 X-AWS-Ease-* 响应头，不污染响应体。
func Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/lambda/", echo("lambda"))
	mux.HandleFunc("/sqs/", echo("sqs"))
	return mux
}

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
