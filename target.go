package awsease

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

var (
	// ErrBadTarget 表示地址为空 / 缺 scheme / lambda 函数名非法 / SQS 消息体非 UTF-8 等
	// —— 调用方一侧的地址/入参问题，重试同样的调用必然同样失败。
	ErrBadTarget = errors.New("awsease: invalid target")
	// ErrUnknownScheme 表示 scheme 不是 http/https/lambda/sqs。
	ErrUnknownScheme = errors.New("awsease: unknown scheme")
	// ErrBadResponse 表示 tunnel 模式下对端响应不是合法的 reqresp 信封
	// —— 对端一侧的问题（函数不是 reqresp 模式、或响应被截断/损坏），与地址合法性无关。
	ErrBadResponse = errors.New("awsease: invalid tunnel response")
)

// parseTarget 把 backend://target[?特性参数] 解析为 (后端, 地址余段, 特性参数)。
//
// 规则（按第一个 "://" 切 scheme 前缀判别后端）：
//   - http/https：整串即 HTTP 请求地址（含 query），原样透传，无特性参数。
//   - lambda    ："?" 前是函数名/ARN，可在第一个 "/" 后附 tunnel 路径（见 doLambda）；
//     "?" 后整段是特性参数。函数名为空、路径为空或含空段（lambda://fn/、fn//x、fn/x/）
//     为非法；特性参数只认 async（true|1|false|0），未知键/值一律拒绝——
//     特性参数现在决定线上格式，拼写错误必须及早报错而非静默改变语义。
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
		fn, path, hasPath := strings.Cut(addr, "/")
		if fn == "" {
			return "", "", nil, fmt.Errorf("awsease: parse url %q: missing function name: %w", target, ErrBadTarget)
		}
		if hasPath {
			for _, seg := range strings.Split(path, "/") {
				if seg == "" {
					return "", "", nil, fmt.Errorf("awsease: parse url %q: empty tunnel path segment: %w", target, ErrBadTarget)
				}
			}
		}
		for key, vals := range feat {
			if key != "async" {
				return "", "", nil, fmt.Errorf("awsease: parse url %q: unknown feature param %q: %w", target, key, ErrBadTarget)
			}
			for _, v := range vals {
				switch v {
				case "true", "1", "false", "0":
				default:
					return "", "", nil, fmt.Errorf("awsease: parse url %q: invalid async value %q: %w", target, v, ErrBadTarget)
				}
			}
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
