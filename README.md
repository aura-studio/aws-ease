# aws-ease

对常用 AWS 调用的便捷封装。核心是一套 **「统一寻址 + 可插拔传输」** 机制：
业务代码只面向一个逻辑目标地址，由库负责把它路由到 HTTP、Lambda 或 SQS。

```bash
go get github.com/aura-studio/aws-ease
```

> 当前已实现 resolver、dispatcher、HTTP / Lambda / SQS transport、本地 mock 服务与统一 Client。
> 设计思路与开发任务清单仍保留在 [doc/TODO.md](doc/TODO.md) 便于追溯。

## 解决什么问题

`lambda.Invoke`、`sqs.SendMessage` 这类 AWS 调用在本地难以运行，通常需要用一个
本地 HTTP 服务来模拟。但「调用 Lambda」还是「请求 HTTP」往往散落在业务代码里，
切换环境就得改代码。

aws-ease 把「调什么」和「怎么调」解耦：

```
调用方  ──►  target 地址  ──►  Resolver 解析 scheme  ──►  Transport 执行
                                                          ├─ http(s)://  → HTTP 请求
                                                          ├─ lambda://   → lambda.Invoke
                                                          └─ sqs://       → SendMessage
```

- 本地开发：通过一条重写规则把 `lambda://*` 指到 `http://localhost:8080`，
  用普通 HTTP 服务即可模拟 Lambda。
- 生产环境：不改业务代码，`lambda://order-service` 直连真实 Lambda 函数。

## 地址约定

| 形式                                   | 路由到      | 说明                                  |
| -------------------------------------- | ----------- | ------------------------------------- |
| `http://host/path` `https://host/path` | HTTP 后端   | 直接发起 HTTP 请求                     |
| `lambda://function-name/path`          | Lambda 后端 | host = 函数名，path/payload 编码后调用 |
| `sqs://queue-name`                     | SQS 后端    | host = 队列名，payload 作为消息体      |

## 状态

| 模块                | 状态 |
| ------------------- | ---- |
| 地址解析 resolver   | ✅   |
| 传输路由 dispatcher | ✅   |
| HTTP transport      | ✅   |
| Lambda transport    | ✅   |
| SQS transport       | ✅   |
| 本地 mock 服务      | ✅   |

## 快速示例

```go
client := awsease.New(
    awsease.WithRewrite("lambda://*", "http://localhost:8080/lambda/{host}{path}"),
)

resp, err := client.Call(context.Background(), "lambda://order-service/create", []byte(`{"id":1}`))
if err != nil {
    panic(err)
}

fmt.Println(resp.StatusCode, string(resp.Body))
```

## 本地联调

启动 mock server：

```bash
go run ./cmd/aws-ease-mock
```

然后把逻辑地址重写到本地：

```go
client := awsease.New(
    awsease.WithRewrite("lambda://*", "http://localhost:8080/lambda/{host}{path}"),
    awsease.WithRewrite("sqs://*", "http://localhost:8080/sqs/{host}"),
)
```

完整任务拆解见 [doc/TODO.md](doc/TODO.md)。

## License

MIT
