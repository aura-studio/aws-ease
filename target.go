package awsease

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

var (
	// ErrBadTarget 表示地址为空 / 缺 scheme / lambda 函数名非法 / SQS 消息体非 UTF-8 等。
	ErrBadTarget = errors.New("awsease: invalid target")
	// ErrUnknownScheme 表示 scheme 不是 http/https/lambda/sqs。
	ErrUnknownScheme = errors.New("awsease: unknown scheme")
)

// parseTarget 把 backend://target[?特性参数] 解析为 (后端, 地址余段, 特性参数)。
//
// 规则（按第一个 "://" 切 scheme 前缀判别后端）：
//   - http/https：整串即 HTTP 请求地址（含 query），原样透传，无特性参数。
//   - lambda    ："?" 前是函数名/ARN（含 "/" 视为非法），"?" 后整段是特性参数。
//   - sqs       ："?" 前是队列名或完整队列 URL，"?" 后整段是特性参数。
//   - 其它      ：ErrUnknownScheme；空串 / 无 "://" / 空 scheme -> ErrBadTarget。
func parseTarget(target string) (backend, string, url.Values, error) {
	i := strings.Index(target, "://")
	if i <= 0 { // 无 "://"、或 scheme 为空（含空串）
		return "", "", nil, fmt.Errorf("awsease: parse url %q: %w", target, ErrBadTarget)
	}
	scheme := strings.ToLower(target[:i]) // RFC 3986：scheme 大小写不敏感
	rest := target[i+len("://"):]

	switch scheme {
	case "http", "https":
		if rest == "" {
			return "", "", nil, fmt.Errorf("awsease: parse url %q: missing host: %w", target, ErrBadTarget)
		}
		// scheme 统一改写为小写（http.Transport 只认小写 scheme），其余整串字节原样作请求地址。
		return backendHTTP, scheme + target[i:], nil, nil
	case "lambda":
		addr, feat, err := splitFeatures(target, rest)
		if err != nil {
			return "", "", nil, err
		}
		if addr == "" {
			return "", "", nil, fmt.Errorf("awsease: parse url %q: missing function name: %w", target, ErrBadTarget)
		}
		if strings.Contains(addr, "/") {
			return "", "", nil, fmt.Errorf("awsease: parse url %q: invalid lambda function name %q: %w", target, addr, ErrBadTarget)
		}
		return backendLambda, addr, feat, nil
	case "sqs":
		addr, feat, err := splitFeatures(target, rest)
		if err != nil {
			return "", "", nil, err
		}
		if addr == "" {
			return "", "", nil, fmt.Errorf("awsease: parse url %q: missing queue: %w", target, ErrBadTarget)
		}
		return backendSQS, addr, feat, nil
	default:
		return "", "", nil, fmt.Errorf("awsease: parse url %q: scheme %q: %w", target, scheme, ErrUnknownScheme)
	}
}

// splitFeatures 按第一个 "?" 把 rest 切成 (地址, 特性参数)。lambda/sqs 的目标本身没有
// query 概念，故 "?" 后整段都是特性参数。
func splitFeatures(target, rest string) (string, url.Values, error) {
	addr, query, _ := strings.Cut(rest, "?")
	feat, err := url.ParseQuery(query)
	if err != nil {
		return "", nil, fmt.Errorf("awsease: parse url %q: bad query: %v: %w", target, err, ErrBadTarget)
	}
	return addr, feat, nil
}
