# aws-ease

对 HTTP / AWS Lambda / AWS SQS 的统一、便捷封装（当前版本 **v0.4.0**）。心智模型只有一句话：

> 给一个 `backend://target` 地址和一段 `payload`，`Invoke` 就把它打到对应后端，返回 `(body, err)`。

```go
resp, err := awsease.Invoke(ctx, "https://api.internal/v1/orders", []byte(`{"sku":"A1"}`))
```

```bash
go get github.com/aura-studio/aws-ease
```

> 可运行示例见 [examples/](examples)（`go run ./examples/localdev` 开箱即跑）；
> [doc/plan.md](doc/plan.md) 是 v0.2 时期的历史设计规格，现行 API 以本 README 与代码为准。

## 解决什么问题

`lambda.Invoke`、`sqs.SendMessage` 这类调用各有一套 SDK、构造、错误处理样板；
「调的是 HTTP 还是 Lambda 还是 SQS」又常硬编码进业务逻辑，切环境就得改代码。

aws-ease 把「调什么」和「怎么调」用一个地址串统一起来：scheme 即后端，
特性参数（Lambda 异步、SQS FIFO/属性）全部以 query 参数写在地址里，一眼可读、可整体进配置（HTTP 后端无特性参数）。

```
调用方 ──► Invoke(ctx, "backend://target?特性参数", payload) ──► scheme 决定后端
                                                                 ├─ http(s):// → net/http 请求
                                                                 ├─ lambda://  → lambda.Invoke
                                                                 └─ sqs://     → SendMessage
```

返回值只有 `(body []byte, err error)`，错误约定也只有一条：**`err` 非 nil 即本次调用失败，此时 `body` 恒为 nil**。
传输失败、地址非法、HTTP 非 2xx、Lambda 函数内部错误，全部走 `err`——
三种后端只需一套 `if err != nil` 处理，重试/日志等横切关注点可以统一封装在 `Invoke` 之上。

## 快速开始

```go
ctx := context.Background()

// 80% 场景：包级 Invoke（默认超时、默认 AWS 凭证链；惰性初始化、并发安全）
body, err := awsease.Invoke(ctx, "https://api.internal/v1/users/42", nil) // HTTP 恒为 POST
body, err = awsease.Invoke(ctx, "lambda://order-create", []byte(`{"sku":"A1"}`))
_, err = awsease.Invoke(ctx, "sqs://order-events", []byte(`{"event":"created"}`)) // 推送成功 body 为 nil

// 需要注入配置（AWS config、mock、超时、本地重定向）时构造 Client，调用形态不变
c := awsease.New(awsease.WithAWSConfig(cfg))
body, err = c.Invoke(ctx, "lambda://order-create", []byte(`{"sku":"A1"}`))
```

## 地址约定

| 地址 | 路由到 | 说明 |
| ---- | ------ | ---- |
| `http://host/path?x=1`、`https://host/path` | HTTP | 整串原样作请求 URL（含 query）；方法恒为 POST，无特性参数 |
| `lambda://<fn>[?async=true]` | Lambda | `<fn>` 是函数名/ARN；payload 原样透传（**无信封**） |
| `sqs://<queue>[?group=g&dedup=d&attr.k=v]` | SQS | `<queue>` 是队列名（惰性解析+缓存）或完整队列 URL |

scheme 大小写不敏感（RFC 3986）：`HTTPS://…`、`Lambda://…` 均合法；
HTTP URL 的 scheme 发出前会统一改写为小写（`http.Transport` 只认小写），URL 其余部分不动。

### HTTP —— `http(s)://…`

整串就是请求 URL，path/query 都写在里面，**字节原样发出**——不重排、不重编码
（`;` 分隔、预编码值、裸键全部原样透传），预签名 URL 等对字节敏感的地址可以放心用。
**方法恒为 POST，无特性参数**（不再有 `ease.` 保留字，不支持自定义方法或请求头）。

返回与错误：2xx 返回响应体字节；**非 2xx 一律返回 `err`**（错误串含 `status <码>` 与响应体，方便直接打日志），`body` 恒为 nil。

```go
body, err := awsease.Invoke(ctx,
    "https://api.internal/v1/users/42?pretty=1", []byte(`{"name":"new"}`))
```

### Lambda —— `lambda://<fn>[?async=true]`

`<fn>` 是函数名或 ARN，含 `/` 视为非法（`ErrBadTarget`，及早报错而非让 AWS 报隐晦错）；
`?` 后整段都是特性参数。payload 原样作 `InvokeInput.Payload`，**无信封**——调用方传什么，Lambda 收什么。

| 特性参数 | 作用 |
| -------- | ---- |
| `async=true`（或 `1`） | `InvocationType=Event` 即发即忘 |

返回与错误：

- 同步成功：返回 Lambda 返回的 payload 字节。
- 函数内部报错（`FunctionError` 非空）：**走 `err`**（错误串含函数错误名与错误 payload），`body` 恒为 nil——
  不伪造假状态码，也不让失败静默成功。
- 异步成功：`(nil, nil)`——仅表示已被 AWS 接受投递，不代表函数已成功执行。

```go
body, err := awsease.Invoke(ctx, "lambda://order-create", []byte(`{"sku":"A1","qty":2}`))
_, err = awsease.Invoke(ctx, "lambda://audit-logger?async=true", payload) // 即发即忘
```

### SQS —— `sqs://<queue>[?group=g&dedup=d&attr.k=v]`

`<queue>` 是队列名（惰性 `GetQueueUrl` 解析并带锁缓存，只解析一次）或完整队列 URL（以 `http` 开头视为已是 URL 直用）；
`?` 后整段都是特性参数。

| 特性参数 | 作用 |
| -------- | ---- |
| `group=<g>` | FIFO `MessageGroupId` |
| `dedup=<d>` | FIFO `MessageDeduplicationId` |
| `attr.<key>=<val>` | String 类型 `MessageAttributes`，可多个；键名原样保留，**不被归一化改写** |

返回与错误：

- 发送成功：`(nil, nil)`——推送语义，MessageId 等回执信息不返回（要它通常也没用，不返回多余信息）。
- payload 非合法 UTF-8：`err`（`errors.Is(err, ErrBadTarget)`）。这是 AWS 对 `MessageBody` 的硬限制，
  库显式校验并报明确错误，而非假装裸字节透传、让 AWS 端报隐晦错。

```go
_, err := awsease.Invoke(ctx,
    "sqs://order-events.fifo?group=orders&dedup=order-7&attr.type=order",
    []byte(`{"event":"created","id":7}`))
```

## 错误约定

只有一条规则：**`err` 非 nil 即失败，此时 `body` 恒为 nil**（不返回半成品，消除「err 非 nil 时 body 还能不能用」的歧义）。

| 失败场景 | 表现 |
| -------- | ---- |
| 地址非法（空串/缺 scheme/lambda 函数名含 `/`） | `err`，`errors.Is(err, ErrBadTarget)` |
| 未知 scheme | `err`，`errors.Is(err, ErrUnknownScheme)` |
| 网络 / AWS SDK / 超时 / context 取消 | `err`（包装底层错误，可 `errors.Is` 透传判别） |
| HTTP 非 2xx | `err`，错误串含 `status <码>` 与响应体 |
| Lambda `FunctionError` 非空 | `err`，错误串含函数错误名与错误 payload |
| SQS payload 非 UTF-8 | `err`，`errors.Is(err, ErrBadTarget)` |

```go
if _, err := awsease.Invoke(ctx, target, payload); errors.Is(err, awsease.ErrUnknownScheme) {
    // scheme 不是 http/https/lambda/sqs
}
```

## Client 与 Options

包级 `Invoke` 用包级默认客户端（默认超时 30s、默认 AWS 凭证链，惰性初始化、并发安全）。
需要注入基础设施时用 `New(opts...)` 构造 `Client`，再调 `Client.Invoke`（签名与包级完全一致）：

```go
c := awsease.New(
    awsease.WithAWSConfig(cfg),
    awsease.WithTimeout(10*time.Second),
)
body, err := c.Invoke(ctx, "lambda://order-create", payload)
```

| Option | 作用 |
| ------ | ---- |
| `WithHTTPClient(h)` | 替换底层 `*http.Client`（transport / 代理 / mTLS / 连接池）。`Timeout` 应留零，超时交给 context 或 `WithTimeout`，避免两个超时源打架 |
| `WithAWSConfig(cfg)` | 注入已加载的 `aws.Config`，共享给 lambda + sqs（生产最常用，一次加载）。不传则首次需要时惰性 `LoadDefaultConfig` |
| `WithLambdaAPI(l)` | 注入 Lambda 客户端实现（测试 mock），签名即 aws-sdk-go-v2 原生形状 |
| `WithSQSAPI(s)` | 注入 SQS 客户端实现（测试 mock） |
| `WithAWSEndpoint(url)` | 为 lambda/sqs 设置 base endpoint（指向 LocalStack 等自建端点） |
| `WithTimeout(d)` | 每次调用的默认超时（默认 30s，内部 `context.WithTimeout`；调用方更短的 deadline 自动优先）。传 0 或负值禁用库级超时，完全交给调用方的 ctx（如 Lambda 同步长任务） |
| `WithLocalRedirect(base)` | 把 `lambda://`、`sqs://` 重定向到 base 的 HTTP mock（本地联调，见下节） |

`New()` 无参即可用：只调 HTTP 时不触发任何 AWS 凭证读取（lambda/sqs 客户端惰性加载）。
`Client` 并发安全（含 SQS QueueUrl 缓存的内部加锁），构造一次、全程复用。

## 本地联调

业务地址串一字不改，构造时加一行把 `lambda://`、`sqs://` 重定向到本地 HTTP mock：

```go
c := awsease.New(awsease.WithLocalRedirect("http://localhost:8080"))
// c.Invoke(ctx, "lambda://order-create", payload) 实际打到 http://localhost:8080/lambda/order-create
// c.Invoke(ctx, "sqs://order-events", payload)    实际打到 http://localhost:8080/sqs/order-events
// http(s):// 地址不受影响
```

重定向语义（与生产对齐，本地验证过的行为切回真实 AWS 不变）：

- 特性参数原样转为重定向 URL 的 query（按键排序），供 mock 端观察；**不**被解读为 HTTP 特性参数——
  例如 `sqs://q?method=DELETE` 里的 `method` 只是透传给 mock 的普通参数，不会改写实际 HTTP 方法
  （重定向请求仍按惯例：payload 非空 POST、为空 GET）。
- 返回值与生产一致：`sqs://` 与 `lambda://…?async=true|1` 成功返回 `(nil, nil)`，不返回 mock 的响应体；
  lambda 同步调用返回 mock 的响应体。
- 校验与生产一致：sqs 在重定向路径同样校验 payload 必须是合法 UTF-8（失败 `ErrBadTarget`）。

启动 mock（`/lambda/`、`/sqs/` 两个 echo 路由，与重定向约定一一对应）：

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
