# examples

每个子目录是一个可独立运行的 `main`。三类后端都用**同一种调用模式**演示，结构绝对统一：

```go
body, err := c.Invoke(ctx, "<scheme>://"+target, []byte(`{"event":"created","id":1}`))
```

`http://` 走 HTTP（payload 非空 -> 默认 POST）、`lambda://` 走 Lambda.Invoke、`sqs://` 走 SQS.SendMessage。
三者的调用、错误处理、结果打印逐行一致，只有 scheme 与客户端构造不同；
err 非 nil 即失败（body 恒为 nil），成功时 body 即响应/返回 payload（发送类后端为 nil）。

| 目录 | 演示 | 是否需要 AWS |
|------|------|--------------|
| [`localdev`](localdev) | `WithLocalRedirect` 把 `lambda://`、`sqs://` 打到进程内 HTTP mock，端到端跑通 + `errors.Is` 哨兵判错 | 否（开箱即跑） |
| [`http`](http) | 包级 `awsease.Invoke(ctx, "http://"+target, payload)` —— 方法恒为 POST、query 原样保留 | 否（需一个可 POST 的端点） |
| [`lambda`](lambda) | `c.Invoke(ctx, "lambda://"+target, payload)` | 是 |
| [`sqs`](sqs) | `c.Invoke(ctx, "sqs://"+target, payload)` | 是 |

## 跑法

```bash
# 完全本地，无需任何凭证：
go run ./examples/localdev

# 三类后端：统一 Invoke 模式，只换 target 来源
AWS_EASE_HTTP_TARGET=httpbin.org/post            go run ./examples/http
AWS_REGION=us-east-1 AWS_EASE_LAMBDA_TARGET=my-func   go run ./examples/lambda
AWS_REGION=us-east-1 AWS_EASE_SQS_TARGET=my-queue     go run ./examples/sqs
```

> 异步 Lambda、FIFO SQS 等细控走 URL 参数，调用模式不变：
> `lambda://fn?async=true`、`sqs://q?group=g&dedup=d&attr.k=v`（HTTP 后端恒为 POST，无特性参数）。
> 完整 API 与 URL 约定见 [../README.md](../README.md)。
