# examples

每个子目录是一个可独立运行的 `main`，演示 aws-ease 的一类用法。

| 目录 | 演示 | 是否需要 AWS |
|------|------|--------------|
| [`localdev`](localdev) | `WithLocalRedirect` 把 `lambda://`、`sqs://` 打到进程内 HTTP mock，端到端跑通且演示错误处理 | 否（开箱即跑） |
| [`http`](http) | 普通 HTTP：`Do` 发 GET、`DoRequest` 自定义 method/header、`OK()` 与错误两层约定 | 否（GET 打 checkip） |
| [`lambda`](lambda) | Lambda：同步 Invoke（Body 即 payload）、`FuncError` 处理、异步 `Async` 即发即忘 | 是 |
| [`sqs`](sqs) | SQS：普通推送取 `MessageID`、FIFO 的 `GroupID`/`DedupID` 与 `Attributes` | 是 |

## 跑法

```bash
# 完全本地，无需任何凭证：
go run ./examples/localdev

# 普通 HTTP（默认对 checkip.amazonaws.com 发 GET）：
go run ./examples/http

# Lambda / SQS 需要 AWS 凭证（环境变量或 ~/.aws）+ 指定目标：
AWS_REGION=us-east-1 AWS_EASE_LAMBDA_FN=my-func   go run ./examples/lambda
AWS_REGION=us-east-1 AWS_EASE_SQS_QUEUE=my-queue  go run ./examples/sqs
```

> 三个概念就够用：地址 `backend://target`、请求 `Request`、响应 `Response`。
> 完整设计见 [../doc/plan.md](../doc/plan.md)。
