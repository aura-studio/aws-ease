package awsease

import (
	"context"
	"fmt"
	"strings"

	"github.com/aura-studio/aws-ease/resolver"
)

// Dispatcher 根据 scheme 将调用路由到对应传输实现。
type Dispatcher struct {
	transports map[string]Transport
}

// NewDispatcher 创建新的分发器。
func NewDispatcher() *Dispatcher {
	return &Dispatcher{transports: map[string]Transport{}}
}

// Register 注册 scheme 对应的传输实现。
func (d *Dispatcher) Register(scheme string, transport Transport) {
	if d.transports == nil {
		d.transports = map[string]Transport{}
	}
	d.transports[strings.ToLower(scheme)] = transport
}

// Call 解析 target 并分发到对应的 Transport。
func (d *Dispatcher) Call(ctx context.Context, target string, payload []byte) (*Response, error) {
	endpoint, err := resolver.Parse(target)
	if err != nil {
		return nil, err
	}

	transport, ok := d.transports[strings.ToLower(endpoint.Scheme)]
	if !ok {
		return nil, fmt.Errorf("no transport registered for scheme %q", endpoint.Scheme)
	}

	return transport.Invoke(ctx, endpoint, payload)
}
