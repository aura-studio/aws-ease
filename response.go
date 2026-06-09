package awsease

import (
	"encoding/json"
	"net/http"
)

// Response 是统一响应。每个后端只填它真正有的字段，其余为零值；
// Backend 标签 + 后端专属字段命名 + 后端感知的 OK() 共同承载诚实，零值不被当成信号。
type Response struct {
	Backend Backend // 本次实际后端，读代码/打日志一眼可读。

	// —— HTTP 专属 ——
	Status int         // HTTP：真实状态码（200/404/500…）。非 HTTP 后端恒为 0，不伪造。
	Header http.Header // HTTP：响应头（多值、标准库类型）。非 HTTP 后端为 nil。

	// —— HTTP / Lambda 共有（都「拿回数据」）——
	Body []byte // HTTP：响应体；Lambda（同步）：返回 payload。SQS / Lambda 异步：nil。

	// —— Lambda 专属 ——
	FuncError string // Lambda 函数内部错误名（如 "Unhandled"）。非空 = 业务级失败。其它后端为 ""。

	// —— SQS 专属 ——
	MessageID string // SQS SendMessage 返回的 MessageId。其它后端为 ""。

	// —— 通用诊断 ——
	Async     bool   // 本次是否即发即忘（Lambda Async）。
	Requested string // 真正打到的地址（经 WithLocalRedirect 重定向后亦记录真实地址），本地联调排错神器。
}

// OK 给出每后端正确的成功判定，调用方不必记三套规则：
//   - HTTP        ：2xx。
//   - Lambda 同步 ：FuncError == ""。
//   - Lambda 异步 ：已被 AWS 接受投递（fire-and-forget，不代表函数已成功执行，见 Async 字段）。
//   - SQS         ：MessageID != ""。
func (r *Response) OK() bool {
	switch r.Backend {
	case BackendHTTP:
		return r.Status >= 200 && r.Status < 300
	case BackendLambda:
		if r.Async {
			return true // 已被 AWS 接受投递（fire-and-forget），不代表函数已成功执行
		}
		return r.FuncError == ""
	case BackendSQS:
		return r.MessageID != ""
	default:
		return false
	}
}

// JSON 便捷反序列化 Body（HTTP 响应体 / Lambda 返回 payload）。
func (r *Response) JSON(v any) error {
	return json.Unmarshal(r.Body, v)
}

// String 返回 string(r.Body)，方便日志与错误信息。
func (r *Response) String() string {
	return string(r.Body)
}
