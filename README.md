# aws-ease

对 HTTP / AWS Lambda / AWS SQS 的统一、便捷封装。心智模型只有一句话：

> 给一个 `backend://target` 地址和一段 `body`，`Do` 就把它打到对应后端，返回一个**自描述的 Response**。

```bash
go get github.com/aura-studio/aws-ease
```

> 可运行示例见 [examples/](examples)（`go run ./examples/localdev` 开箱即跑）；完整设计规格见 [doc/plan.md](doc/plan.md)。

## 解决什么问题

`lambda.Invoke`、`sqs.SendMessage` 这类调用各有一套 SDK、构造、错误处理样板；
「调的是 HTTP 还是 Lambda 还是 SQS」又常硬编码进业务逻辑，切环境就得改代码。

aws-ease 把「调什么」和「怎么调」用一个地址串统一起来：scheme 即后端，一眼可读。

```
调用方 ──► Do(ctx, "backend://target", body) ──► scheme 决定后端
                                                  ├─ http(s):// → net/http 请求
                                                  ├─ lambda://  → lambda.Invoke
                                                  └─ sqs://      → SendMessage
```

## 地址约定

| 地址 | 路由到 | 说明 |
| ---- | ------ | ---- |
| `http://host/path?x=1`、`https://host/path` | HTTP | 整串原样作 URL，path/query 都在里面 |
| `lambda://<function-name-or-arn>` | Lambda | host 段是函数名/ARN；Body 原样作 payload（**无信封**） |
| `sqs://<queue-name>` 或 `sqs://https://.../q` | SQS | host 段是队列名（惰性解析+缓存）或完整队列 URL |

## 快速开始

```go
c := awsease.New(awsease.WithAWSConfig(cfg)) // 只用 HTTP 时 awsease.New() 即可，不碰 AWS 凭证
ctx := context.Background()

// HTTP（Body 空 -> GET）
resp, _ := c.Do(ctx, "https://api.internal/v1/users/42", nil)
fmt.Println(resp.Status, resp.String())

// Lambda 同步 Invoke：Body 即 payload
resp, _ = c.Do(ctx, "lambda://order-create", []byte(`{"sku":"A1"}`))
if resp.FuncError != "" { /* 函数内部报错，诚实暴露 */ }

// SQS 推送：回执在 MessageID
resp, _ = c.Do(ctx, "sqs://order-events", []byte(`{"event":"created"}`))
fmt.Println(resp.MessageID)
```

需要细控（HTTP method/header、Lambda 异步、SQS 属性/FIFO）时用 `DoRequest`：

```go
resp, _ := c.DoRequest(ctx, awsease.Request{
    Target:     "sqs://order-events.fifo",
    Body:       []byte(`{"id":7}`),
    GroupID:    "orders",
    DedupID:    "order-7",
    Attributes: map[string]string{"type": "order"},
})
```

## 诚实的 Response

一个 `Response` 服务三种后端，但**绝不伪造**——每个后端只填它真正有的字段：

| 字段 | HTTP | Lambda | SQS |
| ---- | ---- | ------ | --- |
| `Backend` | `http` | `lambda` | `sqs` |
| `Status` | 真实状态码 | 0 | 0 |
| `Body` | 响应体 | 返回 payload | nil |
| `FuncError` | "" | 非空=函数报错 | "" |
| `MessageID` | "" | "" | 真实 MessageId |

`resp.OK()` 按后端给出正确的成功判定（HTTP 2xx / Lambda `FuncError==""` / SQS 有 MessageID），
调用方不必记三套规则。

**错误约定**：返回的 `error` 仅表示传输层失败（地址非法、网络、AWS SDK、超时），且 `error != nil` 时 `resp == nil`；
业务层失败（HTTP 非 2xx、Lambda `FuncError` 非空）不返回 error，而是 `resp.OK() == false`。

## 本地联调

业务地址串一字不改，构造时加一行把 `lambda://`、`sqs://` 重定向到本地 HTTP mock：

```go
c := awsease.New(awsease.WithLocalRedirect("http://localhost:8080"))
// Do(ctx, "lambda://order-create", body) 实际打到 http://localhost:8080/lambda/order-create
```

启动 mock：

```bash
go run ./cmd/aws-ease-mock
```

## 测试

所有测试集中在 [`tests/`](tests)，一律黑盒（只测公开 API，用注入 fake + `httptest`，不碰真实 AWS）：

```bash
go test ./tests/
```

### 集成测试（真实 AWS，opt-in）

`tests/integration_test.go` 用 build tag `integration` 隔离，默认 `go test`/CI **不会**编译运行。
需要真实 AWS 凭证（环境变量或 `~/.aws`），会自建并清理临时资源（SQS 队列 / Lambda 函数）：

```bash
# HTTP（checkip.amazonaws.com）+ SQS（建队列→发送→收回→删队列）+ Lambda 错误路径
go test -tags integration -run Integration -v ./tests/

# 额外做一次真实 Lambda 成功调用（建一次性 echo 函数→invoke→删除，复用现有执行角色）
AWS_EASE_IT_LAMBDA_CREATE=1 go test -tags integration -run Integration -v ./tests/
```

> 建议用最小权限 IAM 用户（临时 SQS 队列 + `lambda:InvokeFunction`，`AWS_EASE_IT_LAMBDA_CREATE`
> 另需 `lambda:CreateFunction/DeleteFunction` 等），不要用 root key。

## License

MIT
