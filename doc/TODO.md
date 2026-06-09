> ⚠️ **本文件已被 [plan.md](plan.md) 取代（v0.2.0 重构）。**
> 下方内容是 v0.1.0「统一寻址 + 可插拔传输」的历史设计与任务记录，仅作追溯，**不再代表当前架构**。
> 当前架构是「纯 URL-scheme 直连 + 单一 Do/DoRequest + 诚实 Response」，请以 plan.md 为准。

---

# aws-ease 开发任务清单（TODO）（已归档）

> 本文件同时承担两个角色：**设计思路记录** 与 **可执行任务清单**。
>
> 执行约定（给后续会话）：
> - 一次只做一个任务（Task），**完成一个任务就独立提交一次（一个 commit）并推送一次**。
> - commit message 用 `Txx: <简述>` 前缀，对应下面的任务编号，方便追溯。
> - 每个任务都给出了「目标 / 交付物 / 验收标准」，做完请对照验收标准自检。
> - 任务尽量保持顺序依赖最小化；如确需调整顺序或拆分，请在本文件里同步更新。
> - 默认技术栈：Go（module `github.com/aura-studio/aws-ease`，go 1.26.x）、
>   `aws-sdk-go-v2`、函数式 Options 模式、注释用中文，与 aura-studio 其它仓库一致。

---

## 一、设计思路

### 1.1 要解决的痛点

`lambda.Invoke`、`sqs.SendMessage`、`s3.GetObject` 这类 AWS 调用：

1. 在本地开发 / 单元测试时难以直接运行，通常要用本地 HTTP 服务模拟；
2. 「调用的是 Lambda 还是 HTTP 还是发 SQS」这件事，往往硬编码进业务逻辑，
   切换环境（本地 / 测试 / 生产）就得改代码；
3. 各 AWS SDK 的 client 构造、超时、重试、错误处理样板代码重复。

### 1.2 核心抽象：统一寻址 + 可插拔传输

把「**调什么**（目标）」与「**怎么调**（传输）」彻底解耦。

```
                                  ┌────────────► HTTP Transport   ──► net/http 请求
调用方 ──► target 字符串 ──► Resolver ──► Dispatcher ──┼────► Lambda Transport ──► lambda.Invoke
          (逻辑地址)        (解析)      (按 scheme 路由) └────► SQS Transport     ──► SendMessage
```

- **Endpoint**：解析后的结构化地址，至少包含 `Scheme / Host / Path / Query`。
- **Resolver**：把 target 字符串解析为 `Endpoint`，并应用「重写规则」
  （rewrite rules），用于本地把 `lambda://*` 重定向到 `http://localhost`。
- **Transport**：执行真正调用的后端，统一接口
  `Invoke(ctx, Endpoint, payload) (Response, error)`。
- **Dispatcher / Router**：持有 scheme → Transport 的注册表，按 Endpoint.Scheme
  选择 Transport 并调用。对外暴露一个简单入口（如 `Call(ctx, target, payload)`）。

### 1.3 地址约定（初版）

| 形式                            | scheme   | 路由到      | 编码约定（初版）                                  |
| ------------------------------- | -------- | ----------- | ------------------------------------------------- |
| `http://host/path?q`            | `http`   | HTTP        | 直接对该 URL 发请求（默认 POST，可配置）          |
| `https://host/path?q`           | `https`  | HTTP        | 同上                                              |
| `lambda://function-name/path?q` | `lambda` | Lambda      | host=函数名；`{path,query,payload}` 编成 JSON 调用 |
| `sqs://queue-name`              | `sqs`    | SQS         | host=队列名；payload 作为消息体，query 作为属性    |

> Lambda 的 payload 编码要与被调用函数约定好。初版采用一个简单信封
> `{"path": "...", "query": {...}, "payload": <raw>}`，与 aura-studio/lambda
> 仓库的 `reqresp` 风格保持一致，后续可扩展。

### 1.4 本地开发体验

通过 **重写规则** 实现「一行配置切换环境」：

```go
// 本地：把所有 lambda:// 调用打到本地 mock HTTP 服务
r := resolver.New(
    resolver.Rewrite("lambda://*", "http://localhost:8080/lambda/{host}{path}"),
    resolver.Rewrite("sqs://*",    "http://localhost:8080/sqs/{host}"),
)
```

生产环境不加重写规则，`lambda://order-service` 即直连真实 Lambda。

### 1.5 与现有仓库的关系

- `aura-studio/lambda`：已有 http/sqs/reqresp 等 server+client 实现，是被调用端
  （函数侧）。aws-ease 是 **调用端 SDK**，地址/信封编码尽量与其兼容。
- `aura-studio/awsx`：已有的 AWS 小工具（如 sqs 封装），可复用其 client 构造逻辑。
- 实现各 transport 前，先阅读上述两个仓库对应模块，避免重复造轮子、保持协议一致。

---

## 二、任务清单

### T01 — 核心模型：Endpoint 与地址解析 `resolver`

- **目标**：定义结构化地址 `Endpoint` 并实现 target 字符串解析。
- **交付物**：
  - `endpoint.go`：`type Endpoint struct { Scheme, Host, Path string; Query url.Values; Raw string }`。
  - `resolver/resolver.go`：`Parse(target string) (Endpoint, error)`，支持
    `http/https/lambda/sqs` 四种 scheme；非法地址返回明确错误。
  - 表驱动单测，覆盖各 scheme、缺省 path、带 query、非法输入。
- **验收**：`go test ./...` 通过；`Parse("lambda://svc/v1/do?x=1")` 得到
  `Scheme=lambda, Host=svc, Path=/v1/do, Query{x:1}`。

### T02 — Transport 接口与 Dispatcher 路由

- **目标**：定义统一传输接口与按 scheme 路由的分发器。
- **交付物**：
  - `transport.go`：`type Response struct{...}`；
    `type Transport interface { Invoke(ctx, Endpoint, payload []byte) (*Response, error) }`。
  - `dispatcher.go`：scheme→Transport 注册表；`Register(scheme, Transport)`；
    `Call(ctx, target, payload)`（内部 Parse + 选 Transport + Invoke）；
    未注册 scheme 返回明确错误。
  - 用一个内存 fake transport 写单测验证路由正确。
- **验收**：注册 fake 后，`Call("lambda://a", ...)` 命中 lambda transport；
  未注册 scheme 报错。

### T03 — 重写规则（rewrite）支持本地寻址

- **目标**：在解析阶段支持把某 scheme/host 重写为另一目标，服务本地开发。
- **交付物**：
  - `resolver` 增加 `Rewrite(pattern, template)` 选项与匹配逻辑，支持
    `{host}` `{path}` `{query}` 占位符；支持 `scheme://*` 通配。
  - 单测：`lambda://svc/v1/do` 经 `Rewrite("lambda://*","http://localhost:8080/{host}{path}")`
    后解析为 `http://localhost:8080/svc/v1/do`。
- **验收**：重写后 Dispatcher 会路由到 HTTP transport。

### T04 — HTTP transport

- **目标**：实现 HTTP 后端，复用 `aura-studio/httpx` 或标准库。
- **交付物**：
  - `transport/httptrans/`：实现 `Transport`；默认 POST、可配置 method/headers/超时；
    把 `Endpoint.Path+Query` 拼成 URL，payload 作为 body；返回状态码+body。
  - Options 模式配置；用 `httptest.Server` 写单测。
- **验收**：对 httptest server 发请求并正确拿到响应。

### T05 — Lambda transport

- **目标**：实现 Lambda 后端，封装 `lambda.Invoke`。
- **交付物**：
  - `transport/lambdatrans/`：用 `aws-sdk-go-v2/service/lambda`；
    `Endpoint.Host`=函数名；按 1.3 信封编码 payload；处理 `FunctionError`、
    超时、`InvocationType`（同步/异步可选）。
  - client 构造走 `aws-sdk-go-v2/config`，允许注入自定义 endpoint（本地 LocalStack）。
  - 单测：用接口打桩（mock LambdaInvoker）验证编码与错误处理，不依赖真实 AWS。
- **验收**：mock 下信封编码正确、`FunctionError` 被转成 error/Response。

### T06 — SQS transport

- **目标**：实现 SQS 后端，封装 `SendMessage`。
- **交付物**：
  - `transport/sqstrans/`：`Endpoint.Host`=队列名（或别名→QueueURL 映射）；
    payload 作为消息体，`Query` 映射为 MessageAttributes；可选 FIFO 的
    `MessageGroupId/DeduplicationId`。复用 `aura-studio/awsx/sqs` 若合适。
  - 单测：mock SQS 接口验证 SendMessageInput 构造正确。
- **验收**：mock 下 QueueURL 解析与消息体/属性映射正确。

### T07 — 统一客户端门面 `awsease.Client`

- **目标**：把 resolver + dispatcher + 三个 transport 组装成开箱即用的门面。
- **交付物**：
  - 根包 `Client`：`New(opts...)` 默认注册 http/lambda/sqs transport；
    暴露 `Call(ctx, target, payload)` 及便捷方法（如 `Invoke`/`Send`/`Get`/`Post`）。
  - Options：AWS 配置、超时、重写规则、各 transport 自定义。
  - 一个端到端单测（lambda→重写→httptest）走通全链路。
- **验收**：`awsease.New(...).Call(ctx, "lambda://svc/do", body)` 在本地重写下走通。

### T08 — 本地 mock 服务（可选 server 子命令）

- **目标**：提供一个最小 HTTP 服务，模拟 Lambda/SQS 目标，便于本地联调。
- **交付物**：
  - `cmd/aws-ease-mock/` 或 `mockserver/`：接收 `/lambda/{name}{path}` 与
    `/sqs/{queue}`，打印/回显请求，支持注册自定义 handler。
  - README 示例：配合 T03 重写规则本地跑通。
- **验收**：启动后用 Client（带重写）可成功调用并拿到回显。

### T09 — 文档、示例与 CI

- **目标**：补全 README 用法、`example_test.go`、GitHub Actions。
- **交付物**：
  - 更新 `README.md` 各模块状态与可运行示例；
  - `example/` 或 `Example` 测试；
  - `.github/workflows/ci.yml`：`go build` + `go test ./...` + `go vet`。
- **验收**：CI 通过；README 示例可直接复制运行。

### T10 — 打 tag 发首个版本

- **目标**：发布 `v0.1.0`，更新 `awsease.go` 的 `Version`。
- **交付物**：更新 `Version` 常量；`git tag v0.1.0` 并推送。
- **验收**：`go get github.com/aura-studio/aws-ease@v0.1.0` 可用。

---

## 三、后续可扩展（暂不排期）

- 更多 transport：S3（`s3://bucket/key`）、SNS、EventBridge、Step Functions。
- 重试 / 熔断 / 限流中间件（Transport 装饰器链）。
- 可观测性：调用埋点、trace、metrics 钩子。
- 配置文件 / 环境变量驱动的路由表（YAML，与 aura-studio/lambda 的 yml 风格对齐）。
- LocalStack 集成测试。
