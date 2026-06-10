package awsease

import (
	"encoding/json"
	"strconv"
	"strings"
)

// Response 是统一响应：只有状态和有效数据两个字段。
//
// Status 是「HTTP 风格状态码 + 可选附加状态」的字符串，首段恒为三位数字码，
// 空格后是该后端的额外状态返回：
//
//	HTTP        ："200"、"404" …（真实状态码）
//	Lambda 同步 ：成功 "200"；函数内部错误 "500 <FunctionError>"（如 "500 Unhandled"）
//	Lambda 异步 ："202"（已被 AWS 接受投递，不代表函数已成功执行）
//	SQS         ："200 <MessageId>"（回执放在附加状态里）
//
// Payload 是有效数据：HTTP 响应体 / Lambda 返回 payload；SQS 与 Lambda 异步无返回数据，为空串。
type Response struct {
	Status  string
	Payload string
}

// OK 报告本次调用是否成功：Status 首段数字码为 2xx。
func (r *Response) OK() bool {
	code, _, _ := strings.Cut(r.Status, " ")
	n, err := strconv.Atoi(code)
	return err == nil && n >= 200 && n < 300
}

// JSON 便捷反序列化 Payload。
func (r *Response) JSON(v any) error {
	return json.Unmarshal([]byte(r.Payload), v)
}
