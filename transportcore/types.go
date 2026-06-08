package transportcore

import (
	"context"

	"github.com/aura-studio/aws-ease/resolver"
)

// Response 表示统一传输后的响应。
type Response struct {
	StatusCode int
	Body       []byte
	Headers    map[string]string
}

// Transport 抽象具体后端调用方式。
type Transport interface {
	Invoke(ctx context.Context, endpoint resolver.Endpoint, payload []byte) (*Response, error)
}
