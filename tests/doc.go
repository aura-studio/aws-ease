// Package tests 是 aws-ease 的黑盒测试套件：只通过公开 API（Do/DoRequest + Option + Response）
// 验证行为，不依赖任何非导出符号。Lambda/SQS 用注入的 fake 客户端，HTTP 用 httptest，全程不碰真实 AWS。
//
// 真实 AWS 的 opt-in 集成测试见 integration_test.go（//go:build integration）。
package tests
