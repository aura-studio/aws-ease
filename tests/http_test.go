package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	awsease "github.com/aura-studio/aws-ease"
)

// echoView 是 echo 服务器写进响应体的观察结果。新 API 只返回响应体字节、
// 响应头对调用方完全不可见，所以服务器观察到的 method / Path / RawQuery / body
// 都必须经由响应体带回来再断言。
type echoView struct {
	Method string `json:"method"`
	Path   string `json:"path"`  // 服务器看到的 Path（用于断言 path 不被误改写）
	Query  string `json:"query"` // 服务器看到的 RawQuery（用于断言真实 query 原样透传）
	Body   string `json:"body"`  // 服务器收到的请求体
}

// newEchoServer 起一个观察服务器：
//   - /raw      把请求体逐字节原样写回（验证 2xx 返回体字节不被库改写）；
//   - /notfound 返回 404 + 固定提示体（验证非 2xx 的错误语义）；
//   - 其余路径  把观察到的 method / Path / RawQuery / body 编成 JSON 写进响应体。
func newEchoServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		switch r.URL.Path {
		case "/notfound":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("no such page"))
		case "/raw":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body)
		default:
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(echoView{
				Method: r.Method,
				Path:   r.URL.Path,
				Query:  r.URL.RawQuery,
				Body:   string(body),
			})
		}
	}))
}

// invokeEcho 调 Invoke 并把 echo 服务器写在响应体里的观察结果解码回来。
func invokeEcho(t *testing.T, c *awsease.Client, url string, payload []byte) echoView {
	t.Helper()
	body, err := c.Invoke(context.Background(), url, payload)
	if err != nil {
		t.Fatalf("Invoke %s: %v", url, err)
	}
	var v echoView
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("decode echo body %q: %v", body, err)
	}
	return v
}

// TestHTTPAlwaysPost 验证 HTTP 后端方法恒为 POST（无论有无 payload），payload 原样透传。
func TestHTTPAlwaysPost(t *testing.T) {
	srv := newEchoServer()
	defer srv.Close()
	c := awsease.New()

	// 无 payload 也是 POST。
	if v := invokeEcho(t, c, srv.URL+"/", nil); v.Method != http.MethodPost {
		t.Errorf("no-payload method = %q, want POST", v.Method)
	}

	// 有 payload：POST，且 payload 被服务器原样收到。
	v := invokeEcho(t, c, srv.URL+"/", []byte("hello"))
	if v.Method != http.MethodPost {
		t.Errorf("payload method = %q, want POST", v.Method)
	}
	if v.Body != "hello" {
		t.Errorf("server saw body = %q, want hello", v.Body)
	}
}

// TestHTTPRawURLVerbatim 验证 URL 原样发出：path 与预编码的 query 字符（%20、%2F）
// 绝不被重新编码或改写，真实 query 原样到达服务器。
func TestHTTPRawURLVerbatim(t *testing.T) {
	srv := newEchoServer()
	defer srv.Close()
	c := awsease.New()

	const rawQuery = "pre%20encoded=a%2Fb&plain=1"
	v := invokeEcho(t, c, srv.URL+"/x?"+rawQuery, nil)
	if v.Path != "/x" {
		t.Errorf("path = %q, want /x", v.Path)
	}
	if v.Query != rawQuery {
		t.Errorf("query was re-encoded: %q, want %q", v.Query, rawQuery)
	}
}

// TestHTTPNon2xxIsError 验证非 2xx 一律返回 err（含 "status <码>" 与响应体），body 恒为 nil。
func TestHTTPNon2xxIsError(t *testing.T) {
	srv := newEchoServer()
	defer srv.Close()
	c := awsease.New()

	body, err := c.Invoke(context.Background(), srv.URL+"/notfound", nil)
	if err == nil {
		t.Fatal("non-2xx must surface as err")
	}
	if body != nil {
		t.Errorf("body must be nil on non-2xx, got %q", body)
	}
	if !strings.Contains(err.Error(), "status 404") {
		t.Errorf("err %q must contain %q", err, "status 404")
	}
	if !strings.Contains(err.Error(), "no such page") {
		t.Errorf("err %q must carry the response body", err)
	}
}

// TestHTTPBodyBytesVerbatim 验证 2xx 返回体逐字节正确（包含非 UTF-8 字节也不被改写）。
func TestHTTPBodyBytesVerbatim(t *testing.T) {
	srv := newEchoServer()
	defer srv.Close()
	c := awsease.New()

	payload := []byte{0x00, 0xff, 0x10, 'a', 0xfe}
	body, err := c.Invoke(context.Background(), srv.URL+"/raw", payload)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !bytes.Equal(body, payload) {
		t.Errorf("body = %v, want %v (bytes must round-trip verbatim)", body, payload)
	}
}

// TestHTTPStatusClassification 表驱动覆盖状态码分类边界：[200,300) 即成功，其余一律 err
// （错误串含 "status <码>"，body 恒 nil）。用注入 RoundTripper 回放状态码而非 httptest：
// Go 的 ResponseWriter 把 1xx（如 199）当 informational 中间响应发送，真实服务器没法把
// 199 作为终态状态码回给客户端，只有注入才能覆盖到这个下边界。
func TestHTTPStatusClassification(t *testing.T) {
	tests := []struct {
		code    int
		wantErr bool
	}{
		{200, false},
		{201, false},
		{204, false}, // 无内容也是成功：err 为 nil、body 为空
		{299, false}, // 2xx 上边界
		{199, true},  // 1xx 上边界：非 2xx 即失败
		{300, true},  // 3xx 下边界
		{304, true},
		{404, true},
		{500, true},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("status %d", tt.code), func(t *testing.T) {
			respBody := "reply"
			if tt.code == http.StatusNoContent {
				respBody = "" // 204 语义上无响应体
			}
			hc := &http.Client{Transport: rtFunc(func(_ *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: tt.code,
					Body:       io.NopCloser(strings.NewReader(respBody)),
					Header:     make(http.Header),
				}, nil
			})}
			c := awsease.New(awsease.WithHTTPClient(hc))

			body, err := c.Invoke(context.Background(), "http://example.test/", nil)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("status %d must surface as err", tt.code)
				}
				if want := fmt.Sprintf("status %d", tt.code); !strings.Contains(err.Error(), want) {
					t.Errorf("err %q must contain %q", err, want)
				}
				if body != nil {
					t.Errorf("body must be nil on non-2xx, got %q", body)
				}
				return
			}
			if err != nil {
				t.Fatalf("status %d must succeed: %v", tt.code, err)
			}
			if tt.code == http.StatusNoContent {
				if len(body) != 0 {
					t.Errorf("204 body = %q, want empty", body)
				}
				return
			}
			if string(body) != respBody {
				t.Errorf("body = %q, want %q", body, respBody)
			}
		})
	}
}

// TestHTTPSchemeCaseInsensitive 验证 scheme 大小写不敏感（RFC 3986）：大写 scheme 照常
// 路由到 HTTP 后端，发出前被改写为小写。httptest 是明文 http，故用 HTTP:// 大写形式验证。
func TestHTTPSchemeCaseInsensitive(t *testing.T) {
	srv := newEchoServer()
	defer srv.Close()
	c := awsease.New()

	upper := "HTTP://" + strings.TrimPrefix(srv.URL, "http://")
	v := invokeEcho(t, c, upper+"/x?plain=1", nil)
	if v.Method != http.MethodPost {
		t.Errorf("method = %q, want POST", v.Method)
	}
	if v.Query != "plain=1" {
		t.Errorf("query = %q, want plain=1", v.Query)
	}
}

// TestPackageLevelInvoke 验证包级 Invoke（默认客户端）与 Client.Invoke 语义一致。
// 只走 HTTP 后端，确保默认客户端不会触发真实 AWS。
func TestPackageLevelInvoke(t *testing.T) {
	srv := newEchoServer()
	defer srv.Close()

	body, err := awsease.Invoke(context.Background(), srv.URL+"/raw", []byte("ping"))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if string(body) != "ping" {
		t.Errorf("body = %q, want ping", body)
	}
}
