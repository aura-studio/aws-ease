package awsease

import "net/http"

// Request 是细控请求对象。普通结构体，命名字段的 struct literal 已足够自描述，
// 不用 builder，也不用 per-call functional option。只填 Target（+ 多数场景的 Body）即可用；
// 其余零值都是合理默认；字段按后端分组，文档标注哪个字段对哪个后端有效。
type Request struct {
	Target string // backend://target 地址，必填。scheme 决定后端。
	Body   []byte // 请求体 / Lambda payload / SQS 消息体。原样透传，库不包任何信封。

	// —— HTTP 专属（其它后端忽略）——
	// 注意：HTTP 的 path / query 直接写在 Target 里（http://host/path?x=1），不另设字段。
	Method string            // 默认：Body 为空 -> GET，否则 POST。
	Header map[string]string // 请求头。

	// —— Lambda 专属（其它后端忽略）——
	Async bool // true -> InvocationType=Event（即发即忘，见 Response 的诚实表达）。

	// —— SQS 专属（其它后端忽略）——
	Attributes map[string]string // String 类型 MessageAttributes（独立字段，不复用 Header）。
	GroupID    string            // FIFO MessageGroupId。
	DedupID    string            // FIFO MessageDeduplicationId。
}

// httpMethod 返回本次 HTTP 请求实际使用的方法：显式 Method 优先，否则按 Body 是否为空推导
//（无 Body -> GET，有 Body -> POST）。
func (r Request) httpMethod() string {
	if r.Method != "" {
		return r.Method
	}
	if len(r.Body) == 0 {
		return http.MethodGet
	}
	return http.MethodPost
}
