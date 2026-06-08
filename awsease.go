// Package awsease 提供对常用 AWS 调用的便捷封装。
//
// 核心思想是「统一寻址 + 可插拔传输」：调用方只面向一个逻辑目标地址
// （形如 lambda://order-service/v1/create 或 sqs://order-events），
// 由解析器（resolver）将地址拆解为 scheme/host/path，再由对应的传输
// 后端（transport）真正执行——HTTP 后端发起 HTTP 请求、Lambda 后端
// 调用 lambda.Invoke、SQS 后端调用 SendMessage。
//
// 这样同一段业务代码在本地开发时可以把 lambda:// 重写到本地的 HTTP
// mock 服务，在生产环境则直连真实的 AWS Lambda，无需改动调用代码。
//
// 详细的设计思路与开发任务清单见 doc/TODO.md。
package awsease

// Version 是当前模块的语义化版本号。
const Version = "0.1.0"
