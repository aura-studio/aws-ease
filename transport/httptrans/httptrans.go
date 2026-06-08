package httptrans

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/aura-studio/aws-ease/resolver"
	"github.com/aura-studio/aws-ease/transportcore"
)

type Option func(*Transport)

// Transport 通过 HTTP 执行统一调用。
type Transport struct {
	client  *http.Client
	method  string
	headers map[string]string
}

// New 创建 HTTP transport。
func New(opts ...Option) *Transport {
	t := &Transport{
		client: &http.Client{Timeout: 30 * time.Second},
		method: http.MethodPost,
		headers: map[string]string{},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(t)
		}
	}
	return t
}

// WithMethod 配置请求方法。
func WithMethod(method string) Option {
	return func(t *Transport) {
		if method != "" {
			t.method = method
		}
	}
}

// WithHeader 配置默认请求头。
func WithHeader(key, value string) Option {
	return func(t *Transport) {
		if key == "" {
			return
		}
		t.headers[key] = value
	}
}

// WithTimeout 配置 HTTP 客户端超时。
func WithTimeout(timeout time.Duration) Option {
	return func(t *Transport) {
		if timeout > 0 {
			t.client.Timeout = timeout
		}
	}
}

// Invoke 执行 HTTP 请求。
func (t *Transport) Invoke(ctx context.Context, endpoint resolver.Endpoint, payload []byte) (*transportcore.Response, error) {
	req, err := http.NewRequestWithContext(ctx, t.method, endpoint.Raw, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	for key, value := range t.headers {
		req.Header.Set(key, value)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	headers := make(map[string]string, len(resp.Header))
	for key, values := range resp.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}

	return &transportcore.Response{
		StatusCode: resp.StatusCode,
		Body:       body,
		Headers:    headers,
	}, nil
}
