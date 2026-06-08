package resolver

import (
	"fmt"
	"net/url"
	"sort"
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

// Option 配置 Resolver。
type Option func(*Resolver)

// Resolver 负责解析 target 并按规则重写。
type Resolver struct {
	rules []rewriteRule
}

type rewriteRule struct {
	patternScheme string
	template      string
}

// New 创建一个带可选重写规则的 Resolver。
func New(opts ...Option) *Resolver {
	r := &Resolver{}
	for _, opt := range opts {
		if opt != nil {
			opt(r)
		}
	}
	return r
}

// Rewrite 注册一条按 scheme 匹配的重写规则。
func Rewrite(pattern, template string) Option {
	return func(r *Resolver) {
		parts := strings.SplitN(pattern, "://", 2)
		if len(parts) != 2 {
			return
		}
		r.rules = append(r.rules, rewriteRule{
			patternScheme: parts[0],
			template:      template,
		})
	}
}

// Parse 解析 target，并在命中规则时进行重写。
func (r *Resolver) Parse(target string) (Endpoint, error) {
	endpoint, err := Parse(target)
	if err != nil {
		return Endpoint{}, err
	}

	for _, rule := range r.rules {
		if rule.patternScheme != endpoint.Scheme {
			continue
		}

		rewritten := strings.ReplaceAll(rule.template, "{host}", endpoint.Host)
		rewritten = strings.ReplaceAll(rewritten, "{path}", endpoint.Path)
		rewritten = strings.ReplaceAll(rewritten, "{query}", encodeQuery(endpoint.Query))

		return Parse(rewritten)
	}

	return endpoint, nil
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

func encodeQuery(values url.Values) string {
	if len(values) == 0 {
		return ""
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		for _, value := range values[key] {
			parts = append(parts, url.QueryEscape(key)+"="+url.QueryEscape(value))
		}
	}

	return strings.Join(parts, "&")
}
