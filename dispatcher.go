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
	resolver   targetResolver
}

type targetResolver interface {
	Parse(target string) (resolver.Endpoint, error)
}

// DispatcherOption 配置 Dispatcher。
type DispatcherOption func(*Dispatcher)

// NewDispatcher 创建新的分发器。
func NewDispatcher(opts ...DispatcherOption) *Dispatcher {
	d := &Dispatcher{
		transports: map[string]Transport{},
		resolver:   resolver.New(),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(d)
		}
	}
	return d
}

// WithResolver 为 Dispatcher 注入自定义 Resolver。
func WithResolver(res targetResolver) DispatcherOption {
	return func(d *Dispatcher) {
		if res != nil {
			d.resolver = res
		}
	}
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
	endpoint, err := d.resolver.Parse(target)
	if err != nil {
		return nil, err
	}

	transport, ok := d.transports[strings.ToLower(endpoint.Scheme)]
	if !ok {
		return nil, fmt.Errorf("no transport registered for scheme %q", endpoint.Scheme)
	}

	return transport.Invoke(ctx, endpoint, payload)
}
