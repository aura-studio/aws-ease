package awsease

import (
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrBadTarget 表示地址为空 / 缺 scheme / lambda 函数名非法等。
	ErrBadTarget = errors.New("awsease: invalid target")
	// ErrUnknownScheme 表示 scheme 不是 http/https/lambda/sqs。
	ErrUnknownScheme = errors.New("awsease: unknown scheme")
)

// parseTarget 把 backend://target 地址解析为 (后端, 地址余段)。
//
// 规则（按第一个 "://" 切 scheme 前缀判别后端，余下整段按后端落位）：
//   - http/https：保留整串（含 scheme）作为 HTTP URL，不二次拆 host/path/query。
//   - lambda    ：前缀后整段是函数名/ARN（含 "/" 或 "?" 视为非法 -> ErrBadTarget）。
//   - sqs       ：前缀后整段是队列名或完整队列 URL（其本身可再是一个 https:// URL）。
//   - 其它      ：ErrUnknownScheme；空串 / 无 "://" / 空 scheme -> ErrBadTarget。
func parseTarget(target string) (Backend, string, error) {
	i := strings.Index(target, "://")
	if i <= 0 { // 无 "://"、或 scheme 为空（含空串）
		return "", "", fmt.Errorf("awsease: parse target %q: %w", target, ErrBadTarget)
	}
	scheme := target[:i]
	rest := target[i+len("://"):]

	switch scheme {
	case "http", "https":
		if rest == "" {
			return "", "", fmt.Errorf("awsease: parse target %q: missing host: %w", target, ErrBadTarget)
		}
		return BackendHTTP, target, nil // 整串（含 scheme）原样作 URL
	case "lambda":
		if rest == "" {
			return "", "", fmt.Errorf("awsease: parse target %q: missing function name: %w", target, ErrBadTarget)
		}
		if strings.ContainsAny(rest, "/?") {
			return "", "", fmt.Errorf("awsease: parse target %q: invalid lambda function name %q: %w", target, rest, ErrBadTarget)
		}
		return BackendLambda, rest, nil
	case "sqs":
		if rest == "" {
			return "", "", fmt.Errorf("awsease: parse target %q: missing queue: %w", target, ErrBadTarget)
		}
		return BackendSQS, rest, nil
	default:
		return "", "", fmt.Errorf("awsease: parse target %q: scheme %q: %w", target, scheme, ErrUnknownScheme)
	}
}

// redirectTarget 在设置了 WithLocalRedirect 时，把 lambda://、sqs:// 改写为对本地 base 的 HTTP 调用：
//
//	lambda://<fn>    -> {base}/lambda/<fn>
//	sqs://<queue>    -> {base}/sqs/<queue>
//
// http(s):// 不受影响。返回改写后的 (后端, 地址)；未配置或无需改写时原样返回。
func (c *Client) redirectTarget(b Backend, addr string) (Backend, string) {
	if c.redirectBase == "" {
		return b, addr
	}
	base := strings.TrimRight(c.redirectBase, "/")
	switch b {
	case BackendLambda:
		return BackendHTTP, base + "/lambda/" + addr
	case BackendSQS:
		return BackendHTTP, base + "/sqs/" + addr
	default:
		return b, addr
	}
}
