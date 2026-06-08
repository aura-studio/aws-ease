package resolver

import (
	"fmt"
	"net/url"
	"strings"
)

// Endpoint 表示解析后的目标地址。
type Endpoint struct {
	Scheme string
	Host   string
	Path   string
	Query  url.Values
	Raw    string
}

var supportedSchemes = map[string]struct{}{
	"http":   {},
	"https":  {},
	"lambda": {},
	"sqs":    {},
}

// Parse 将目标地址解析为结构化 Endpoint。
func Parse(target string) (Endpoint, error) {
	parsed, err := url.Parse(target)
	if err != nil {
		return Endpoint{}, fmt.Errorf("parse target %q: %w", target, err)
	}

	if parsed.Scheme == "" {
		return Endpoint{}, fmt.Errorf("parse target %q: missing scheme", target)
	}
	if _, ok := supportedSchemes[parsed.Scheme]; !ok {
		return Endpoint{}, fmt.Errorf("parse target %q: unsupported scheme %q", target, parsed.Scheme)
	}
	if parsed.Host == "" {
		return Endpoint{}, fmt.Errorf("parse target %q: missing host", target)
	}

	path := parsed.EscapedPath()
	if path == "" {
		path = "/"
	} else if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	query := parsed.Query()
	if query == nil {
		query = url.Values{}
	}

	return Endpoint{
		Scheme: parsed.Scheme,
		Host:   parsed.Host,
		Path:   path,
		Query:  query,
		Raw:    target,
	}, nil
}
