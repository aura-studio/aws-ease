# aws-ease

对常用 AWS 调用的便捷封装。核心是一套 **「统一寻址 + 可插拔传输」** 机制：
业务代码只面向一个逻辑目标地址，由库负责把它路由到 HTTP、Lambda 或 SQS。

```bash
go get github.com/aura-studio/aws-ease
```

> 🚧 本仓库处于脚手架阶段。整体设计思路与开发任务清单见 [doc/TODO.md](doc/TODO.md)。
> 各功能模块会按任务逐个实现，本 README 会随之补全用法示例。

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

## 地址约定（草案）

| 形式                                   | 路由到      | 说明                                  |
| -------------------------------------- | ----------- | ------------------------------------- |
| `http://host/path` `https://host/path` | HTTP 后端   | 直接发起 HTTP 请求                     |
| `lambda://function-name/path`          | Lambda 后端 | host = 函数名，path/payload 编码后调用 |
| `sqs://queue-name`                     | SQS 后端    | host = 队列名，payload 作为消息体      |

## 状态

| 模块                | 状态 |
| ------------------- | ---- |
| 地址解析 resolver   | TODO |
| 传输路由 dispatcher | TODO |
| HTTP transport      | TODO |
| Lambda transport    | TODO |
| SQS transport       | TODO |
| 本地 mock 服务      | TODO |

完整任务拆解见 [doc/TODO.md](doc/TODO.md)。

## License

MIT
