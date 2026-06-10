# examples

每个子目录是一个可独立运行的 `main`。三类后端都用**同一种调用模式**演示，结构绝对统一：

```go
resp, err := c.Do(ctx, "<scheme>://"+target, []byte(`{"event":"created","id":1}`))
```

`http://` 走 HTTP（Body 非空 -> 默认 POST、默认 header）、`lambda://` 走 Lambda.Invoke、`sqs://` 走 SQS.SendMessage。
三者的调用、错误处理、结果打印逐行一致，只有 scheme 与客户端构造不同。

| 目录 | 演示 | 是否需要 AWS |
|------|------|--------------|
| [`localdev`](localdev) | `WithLocalRedirect` 把 `lambda://`、`sqs://` 打到进程内 HTTP mock，端到端跑通 + 错误处理 | 否（开箱即跑） |
| [`http`](http) | `c.Do(ctx, "http://"+target, payload)` —— 默认 POST、默认 header | 否（需一个可 POST 的端点） |
| [`lambda`](lambda) | `c.Do(ctx, "lambda://"+target, payload)` | 是 |
| [`sqs`](sqs) | `c.Do(ctx, "sqs://"+target, payload)` | 是 |

## 跑法

```bash
# 完全本地，无需任何凭证：
go run ./examples/localdev

# 三类后端：统一 c.Do 模式，只换 target 来源
AWS_EASE_HTTP_TARGET=httpbin.org/post            go run ./examples/http
AWS_REGION=us-east-1 AWS_EASE_LAMBDA_TARGET=my-func   go run ./examples/lambda
AWS_REGION=us-east-1 AWS_EASE_SQS_TARGET=my-queue     go run ./examples/sqs
```

> 异步 Lambda、FIFO SQS、自定义 HTTP method/header 等细控用 `DoRequest`；本目录只演示统一的 `Do`。
> 完整设计见 [../doc/plan.md](../doc/plan.md)。
