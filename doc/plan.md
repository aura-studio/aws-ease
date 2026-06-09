# aws-ease 最终设计规格（v0.2.0 重构）

> 本文是重构的唯一蓝图。它经过「三套设计 → 对抗式评审 → 综合」得出诚实的 Response 模型与删除清单，
> 并按**用户拍板的「纯 URL-scheme 直连寻址」**重新定型。立场强硬：**一切为「易用 + 易懂」服务，
> 旧特性可全部推翻，不为兼容妥协。**
>
> 寻址决策：**业务代码直接写 `backend://target` 字符串**，库按 scheme 路由到 HTTP / Lambda / SQS。
> 零路由表、零配置即可调用任意目标；本地/生产切换靠换传入的串（或一条 `WithLocalRedirect` 开关）。

---

## 1. 设计理念与电梯陈述

**一句话**：给一个 `backend://target` 地址和一段 `body`，`Do` 就把它打到对应后端，
返回一个**自描述的 `Response`**——`Backend` 标签 + 后端专属字段诚实表达 HTTP / Lambda / SQS 三种语义差异，
绝不给非 HTTP 后端伪造 HTTP 状态码。

**三条信条（强硬取舍）：**

1. **scheme 即后端，地址即一切。** `http(s)://` 走 HTTP、`lambda://` 走 Lambda.Invoke、`sqs://` 走 SQS.SendMessage。
   后端选择写在地址里、一眼可读，不藏进隐式约定。**因为 scheme 显式声明了后端，「错配静默失败」从根上不可能发生**
   ——这让旧设计里需要的「后端断言/路由校验」一并消失（概念更少 = 更易懂）。

2. **诚实 = 不伪造。** 只用**一个** `Response`（统一接口），但**绝不给非 HTTP 后端伪造 HTTP 状态码**。
   `Status` 只在 HTTP 后端有值；Lambda 的函数错误用 `FuncError` 字段表达，SQS 的回执用 `MessageID` 字段表达。
   `Backend` 标签 + 后端专属字段 + 后端感知的 `OK()` 三者共同承载「这次成功了吗」，零值不再被当作信号。

3. **极简门面，零配置可用。** `awsease.New()` 无参即可用；只调 HTTP 时不碰任何 AWS 凭证。
   主入口 `Do(ctx, target, body)` 是一行的 80% 场景；需要细控（HTTP method/header、Lambda 异步、SQS 属性/FIFO）
   时用 `DoRequest(ctx, Request{...})`。两者都返回统一的 `(*Response, error)`，
   让「对任意后端做统一重试/日志/中间件」成为可能。

---

## 2. 最终公开 API（可编译级精确）

> 单包 `awsease`，无子包、无 alias 文件、无 `transportcore`、无 `dispatcher`/`resolver` 子包。
> 下列代码为公开契约的精确形状（实现体省略为注释）。

```go
// Package awsease 提供对 HTTP / AWS Lambda / AWS SQS 的统一、便捷封装。
//
// 心智模型只有「地址 + Body -> Response」：
//   - 地址是 backend://target 字符串，scheme 决定后端（http/https/lambda/sqs）。
//   - Response 用 Backend 标签 + 后端专属字段诚实区分三种语义。
//
// 主入口 Client.Do(ctx, target, body)；需要细控时用 Client.DoRequest(ctx, Request)。
// 本地/生产切换靠换地址串，或 New(WithLocalRedirect(base))。
package awsease

import (
	"context"
	"errors"
	"net/http"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
)

// Version 是当前模块语义化版本号。
const Version = "0.2.0"

// ─────────────────────────────────────────────────────────────────────────
// 后端类型
// ─────────────────────────────────────────────────────────────────────────

// Backend 是后端类型，三选一。它是 Response 的「自解释标签」，由地址 scheme 推导。
type Backend string

const (
	BackendHTTP   Backend = "http"   // http:// 或 https://，标准 HTTP 请求/响应。
	BackendLambda Backend = "lambda" // lambda://<fn>，lambda.Invoke（同步取 payload，或异步 Event 即发即忘）。
	BackendSQS    Backend = "sqs"    // sqs://<queue>，sqs.SendMessage（推送取 MessageId）。
)

// ─────────────────────────────────────────────────────────────────────────
// 客户端
// ─────────────────────────────────────────────────────────────────────────

// Client 是唯一门面，并发安全（含 SQS QueueUrl 缓存的内部加锁）。零值不可用，必须经 New 构造。
type Client struct {
	// unexported：httpClient / lambda / sqs / awsCfg(+once) / defaultTimeout / redirectBase /
	//             queueURLCache + sync.RWMutex ...
}

// New 创建客户端。无参即可用（HTTP 立即可用；lambda/sqs 的真实 AWS 客户端惰性加载，
// 只用 HTTP 时不会触发任何 AWS 凭证读取）。
func New(opts ...Option) *Client

// Do 是主入口：解析 target 的 scheme -> 选后端 -> 执行 -> 返回统一 Response。
// 等价于 DoRequest(ctx, Request{Target: target, Body: body})。
//
//   c.Do(ctx, "https://api.internal/v1/users/42", nil)        // HTTP GET（body 空）
//   c.Do(ctx, "lambda://order-create", payload)               // Lambda 同步 Invoke
//   c.Do(ctx, "sqs://order-events", msg)                      // SQS 推送
//
// 错误约定（见第 9 章）：error 仅表示传输层失败，且 error 非 nil 时 resp 为 nil；
// 业务层失败（HTTP 非 2xx / Lambda FuncError 非空）不返回 error，而是 resp.OK()==false。
func (c *Client) Do(ctx context.Context, target string, body []byte) (*Response, error)

// DoRequest 是带细粒度控制的入口（HTTP method/header、Lambda 异步、SQS 属性/FIFO）。
func (c *Client) DoRequest(ctx context.Context, req Request) (*Response, error)

// ─────────────────────────────────────────────────────────────────────────
// 请求
// ─────────────────────────────────────────────────────────────────────────

// Request 是细控请求对象。普通结构体，命名字段的 struct literal 已足够自描述，不用 builder，
// 也不用 per-call functional option（避免「无关后端 option 被静默忽略」的零反馈 footgun）。
// 只填 Target（+ 多数场景的 Body）即可用；其余零值都是合理默认；按后端分组、文档标注哪个字段对哪个后端有效。
type Request struct {
	Target string // backend://target 地址，必填。scheme 决定后端。
	Body   []byte // 请求体 / Lambda payload / SQS 消息体。原样透传，库不包任何信封。

	// —— HTTP 专属（其它后端忽略）——
	// 注意：HTTP 的 path / query 直接写在 Target 里（http://host/path?x=1），不另设字段。
	Method string            // 默认：Body 为空 -> GET，否则 POST。
	Header map[string]string // 请求头。

	// —— Lambda 专属（其它后端忽略）——
	Async bool // true -> InvocationType=Event（即发即忘，见 Response 的诚实表达）。

	// —— SQS 专属（其它后端忽略）——
	Attributes map[string]string // String 类型 MessageAttributes（独立字段，不复用 Header，见 3.4）。
	GroupID    string            // FIFO MessageGroupId。
	DedupID    string            // FIFO MessageDeduplicationId。
}

// ─────────────────────────────────────────────────────────────────────────
// 响应（诚实表达三后端差异：Backend 标签 + 后端专属字段，绝不伪造）
// ─────────────────────────────────────────────────────────────────────────

// Response 是统一响应。每个后端只填它真正有的字段，其余为零值；
// Backend 标签 + 后端专属字段命名 + 后端感知的 OK() 共同承载诚实，零值不被当成信号。
type Response struct {
	Backend Backend // 本次实际后端，读代码/打日志一眼可读。

	// —— HTTP 专属 ——
	Status int         // HTTP：真实状态码（200/404/500…）。**非 HTTP 后端恒为 0，不伪造。**
	Header http.Header // HTTP：响应头（多值、标准库类型）。非 HTTP 后端为 nil。

	// —— HTTP / Lambda 共有（都「拿回数据」）——
	Body []byte // HTTP：响应体；Lambda（同步）：返回 payload。SQS / Lambda 异步：nil。

	// —— Lambda 专属 ——
	FuncError string // Lambda 函数内部错误名（如 "Unhandled"）。非空 = 业务级失败。其它后端为 ""。

	// —— SQS 专属 ——
	MessageID string // SQS SendMessage 返回的 MessageId。其它后端为 ""。

	// —— 通用诊断 ——
	Async     bool   // 本次是否即发即忘（Lambda Async）。
	Requested string // 真正打到的地址（经 WithLocalRedirect 重定向后亦记录真实地址），本地联调排错神器。
}

// OK 给出**每后端正确**的成功判定，调用方不必记三套规则：
//   - HTTP   ：2xx。
//   - Lambda 同步：FuncError == ""。
//   - Lambda 异步：投递已被 AWS 接受（fire-and-forget，不代表函数已成功执行，见 Async 字段）。
//   - SQS    ：MessageID != ""。
func (r *Response) OK() bool

// JSON 便捷反序列化 Body（HTTP 响应体 / Lambda 返回 payload）。
func (r *Response) JSON(v any) error

// String 返回 string(r.Body)，方便日志与错误信息。
func (r *Response) String() string

// ─────────────────────────────────────────────────────────────────────────
// 构造期 Option
// ─────────────────────────────────────────────────────────────────────────

type Option func(*config)

// WithHTTPClient 替换底层 *http.Client（transport / 代理 / mTLS / 连接池）。
// 注意：超时请用 context 或 WithTimeout，注入的 http.Client.Timeout 应留零，避免与 context 超时打架。
func WithHTTPClient(h *http.Client) Option

// WithAWSConfig 注入已加载的 aws.Config，共享给 lambda + sqs（生产最常用，一次加载）。
// 不传则首次需要时惰性 config.LoadDefaultConfig。
func WithAWSConfig(cfg awssdk.Config) Option

// WithLambdaAPI 注入 Lambda 客户端实现（测试 mock）。签名即 aws-sdk-go-v2 原生形状。
func WithLambdaAPI(l LambdaAPI) Option

// WithSQSAPI 注入 SQS 客户端实现（测试 mock）。
func WithSQSAPI(s SQSAPI) Option

// WithAWSEndpoint 为 lambda/sqs 设置 base endpoint（指向 LocalStack 等自建端点）。
func WithAWSEndpoint(url string) Option

// WithTimeout 设置每次调用的默认超时（内部以 context.WithTimeout 套在传入 ctx 上）。默认 30s。
// 调用方可用更短的 context deadline 覆盖（context 自动取更早者）。
func WithTimeout(d time.Duration) Option

// WithLocalRedirect 是 URL-scheme 下的本地切换开关（可选）：设置后，所有 lambda:// 与 sqs:// 调用
// 被改写为对 base 的 HTTP 请求，按 cmd/aws-ease-mock 的约定落到 base/lambda/<fn> 与 base/sqs/<queue>；
// http(s):// 地址不受影响。这是唯一、极简、无模板 DSL 的本地重定向（取代旧 WithRewrite 的 {host}{path} 模板）。
//   awsease.New(awsease.WithLocalRedirect("http://localhost:8080"))
func WithLocalRedirect(base string) Option

// ─────────────────────────────────────────────────────────────────────────
// 可注入接口（就近定义，不再单列 transportcore；签名即 aws-sdk-go-v2 原生形状）
// ─────────────────────────────────────────────────────────────────────────

type LambdaAPI interface {
	Invoke(ctx context.Context, in *awslambda.InvokeInput, optFns ...func(*awslambda.Options)) (*awslambda.InvokeOutput, error)
}

type SQSAPI interface {
	SendMessage(ctx context.Context, in *awssqs.SendMessageInput, optFns ...func(*awssqs.Options)) (*awssqs.SendMessageOutput, error)
	GetQueueUrl(ctx context.Context, in *awssqs.GetQueueUrlInput, optFns ...func(*awssqs.Options)) (*awssqs.GetQueueUrlOutput, error)
}

// ─────────────────────────────────────────────────────────────────────────
// 哨兵错误
// ─────────────────────────────────────────────────────────────────────────

var (
	ErrBadTarget     = errors.New("awsease: invalid target")  // 地址为空 / 缺 scheme / lambda 函数名非法等。
	ErrUnknownScheme = errors.New("awsease: unknown scheme")  // scheme 不是 http/https/lambda/sqs。
)
```

> **关于注入接口可见性的取舍：** 公开 `LambdaAPI/SQSAPI`（aws-sdk 原生形状）而非非导出窄接口。
> 理由：测试时 fake 那几个方法即可，签名就是 SDK 原方法，最地道、最低惊讶；
> 代价是只用 HTTP 的消费者编译期也会拖入 aws-sdk 依赖——但本模块名为 aws-ease、就是为 AWS 而生，
> 这个张力可接受，且文档点明。

---

## 3. 寻址规范（纯 URL-scheme 直连）

### 3.1 地址格式 = `backend://target`

业务代码**直接写地址串**，scheme 即后端，零路由表、零注册、可调任意目标。

| 地址形式 | 后端 | 如何被定位 |
|----------|------|------------|
| `http://host/path?x=1`、`https://host/path` | HTTP | 整串原样交 `http.NewRequestWithContext`；path / query 都在地址里。 |
| `lambda://<function-name-or-arn>` | Lambda | `://` 之后整段 = `InvokeInput.FunctionName`（函数名或 ARN，原样取用）。 |
| `sqs://<queue-name>` 或 `sqs://https://sqs.../q` | SQS | `://` 之后整段：以 `http` 开头当 QueueUrl 直用；否则当队列名，惰性 `GetQueueUrl` 解析并带锁缓存。 |

### 3.2 解析规则（约 20 行，无 url.Parse 全量拆解、无 DSL、无正则）

按**第一个 `://`** 切出 scheme 前缀判别后端，余下整段按上表落位：

- `http` / `https`：**保留整串**（含 scheme）作为 HTTP URL，不二次拆 host/path/query。
- `lambda`：前缀后整段是函数名/ARN（含 `/` 视为非法函数名 -> `ErrBadTarget`，及早报错而非让 AWS 报隐晦错）。
- `sqs`：前缀后整段是队列名或完整队列 URL（其本身可以再是一个 `https://` URL，故**不**用 `url.Parse` 解析外层）。
- 其它 scheme -> `ErrUnknownScheme`；空串 / 无 `://` -> `ErrBadTarget`。

> 不引入 `Endpoint{Scheme,Host,Path,Query,Raw}` 结构、不引入 `Rewrite` 模板语言——
> 这些是旧 `resolver` 包的复杂度来源，整包删除。

### 3.3 Lambda payload 编码约定（明确：**不保留信封**）

**直接把 `Body` 原样作为 `InvokeInput.Payload`，不再包 `{"path","query","payload"}` 信封。**

这是对旧实现（`transport/lambdatrans/lambdatrans.go` 第 23–27、89–93 行强制信封）最核心的修正——
旧信封是调用方看不见的私有耦合约定。新约定「调用方传什么，Lambda 收什么，所见即所得」。
若函数确需结构信息，调用方自行放进 `Body` 的 JSON 里，**显式优于隐藏**。

### 3.4 SQS body 与属性的诚实约束

- **SQS body 不是任意裸字节**：`SendMessage.MessageBody` 是 `string`，仅接受合法 UTF-8。
  实现里对 `Body` 做 UTF-8 校验，非法时返回明确错误（`ErrBadTarget` 同级），而非让 AWS 端报隐晦错。
- **SQS 属性用独立字段 `Request.Attributes map[string]string`**，不复用 HTTP 的 `Header`。
  理由：`http.Header` 会经 `CanonicalMIMEHeaderKey` 改写键名大小写，且无法表达 SQS 的 Number/Binary DataType。
  当前只支持 String 属性（覆盖绝大多数场景），用独立 `map[string]string` 避免键名被悄悄改写。
  （Number/Binary 属性属非目标，见第 11 章。）

---

## 4. Response 模型（逐字段 × 三后端填充）

**核心原则：诚实 = 不伪造 + Backend 标签 + 后端专属字段 + 后端感知 OK()。零值不被当作信号。**

| 字段 | HTTP | Lambda（同步） | Lambda（Async） | SQS |
|------|------|----------------|-----------------|-----|
| `Backend` | `http` | `lambda` | `lambda` | `sqs` |
| `Status` | 真实状态码 | **0**（不伪造） | **0** | **0**（不伪造 202） |
| `Header` | 响应头 | nil | nil | nil |
| `Body` | 响应体 | 返回 payload | nil | nil |
| `FuncError` | "" | 非空=函数内部报错 | "" | "" |
| `MessageID` | "" | "" | "" | 真实 MessageId |
| `Async` | false | false | true | false |
| `Requested` | 实打到的 URL | 函数名/ARN | 函数名/ARN | 解析后的 QueueUrl |
| `OK()` | 2xx | `FuncError==""` | 已投递=true | `MessageID!=""` |

### 4.1 关键诚实化决策

1. **`Status` 对非 HTTP 后端恒为 0，绝不伪造。** 「这次成功了吗」由 `OK()` 回答，不由一个假 HTTP 码回答。
2. **Lambda 函数错误用 `FuncError` 表达，不映射成假 502，也不返回 transport error。**
   AWS 返回 `200 + FunctionError` 是**应用级**失败：`FuncError` 非空、`OK()==false`、`Body` 仍带回函数给的
   错误 payload 供排查、**`error` 为 nil**。调用方：`if resp.FuncError != "" {…}` 或统一 `if !resp.OK() {…}`。
3. **只用一个 `Response`，不为 SQS 拆 `Ack` 类型。** 统一接口让「对任意后端统一中间件」可行；
   SQS 诚实靠 `MessageID` 专属字段 + `Backend==sqs` + `OK()` 判 `MessageID!=""`，而非伪造 `Status=202`。
4. **异步 Lambda 的成功语义被诚实建模。** 异步时 `Async==true`、`Body==nil`、`Status==0`，
   `OK()` 仅表示「已被 AWS 接受投递」，配合 `Async` 字段与文档明示「不代表函数已执行成功」。

---

## 5. Options 清单

### 5.1 构造期 Option（作用于整个 Client，基础设施级，构造一次）

| Option | 作用 |
|--------|------|
| `WithHTTPClient(h)` | 替换 `*http.Client`：transport / 代理 / mTLS / 连接池（Timeout 留零）。 |
| `WithAWSConfig(cfg)` | 注入已加载 `aws.Config`，共享给 lambda+sqs（生产最常用）。 |
| `WithLambdaAPI(l)` | 注入 Lambda 实现（测试 mock）。 |
| `WithSQSAPI(s)` | 注入 SQS 实现（测试 mock）。 |
| `WithAWSEndpoint(url)` | lambda/sqs base endpoint（LocalStack 端到端）。 |
| `WithTimeout(d)` | 每次调用默认超时（默认 30s，可被更短的 context deadline 覆盖）。 |
| `WithLocalRedirect(base)` | 把 lambda://、sqs:// 重定向到 base 的 HTTP mock（本地切换，可选）。 |

**删除的旧 Option（不保留）：** `WithRewrite(pattern,template)`（模板 DSL）、
`WithHTTPTransportOptions` / `WithLambdaTransportOptions` / `WithSQSTransportOptions`（洋葱式子包 Option 转发）。

### 5.2 per-call 配置（`Request` 字段，每次变化的请求内容）

| 字段 | 后端 | 作用 |
|------|------|------|
| `Target` | 全部 | `backend://target` 地址，必填。 |
| `Body` | 全部 | 请求体 / payload / 消息体。 |
| `Method` | HTTP | 默认 Body 空=GET 否则 POST。 |
| `Header` | HTTP | 请求头。 |
| `Async` | Lambda | `InvocationType=Event` 即发即忘。 |
| `Attributes` | SQS | String 类型 MessageAttributes（独立 map，不被键名归一化）。 |
| `GroupID` | SQS FIFO | MessageGroupId。 |
| `DedupID` | SQS FIFO | MessageDeduplicationId。 |

**原则：** 基础设施进 `Option`（构造一次、跨调用稳定）；请求内容进 `Request` struct（每次变）。
两类物理分离，杜绝旧实现「Option 既配 transport 又配 rewrite」的混杂。
**不引入 per-call functional option**——`Request` struct literal 更地道更易读，
且避免「无关后端 option 被静默忽略」的零反馈 footgun（字段按后端分组、文档标注谁对谁有效）。

---

## 6. 本地开发 / 测试切换 + mock

### 6.1 本地切换：两种都行，业务的 `Do` 调用形态不变

**法一（推荐，零库特性）——地址串本身由 app 配置决定：** 生产配置给 `lambda://order-create`，
本地配置给 `http://localhost:8080/lambda/order-create`。库不介入，最诚实。

**法二（便捷开关）——`WithLocalRedirect`：** 一行把所有 lambda://、sqs:// 打到本地 HTTP mock，
业务代码里的地址串一字不改：
```go
// 生产
c := awsease.New(awsease.WithAWSConfig(cfg))
// 本地：同样的 Do("lambda://order-create", …) 现在落到 http://localhost:8080/lambda/order-create
c := awsease.New(awsease.WithLocalRedirect("http://localhost:8080"))
```
重定向约定与 `cmd/aws-ease-mock` 对齐：`lambda://<fn>` -> `{base}/lambda/<fn>`，`sqs://<q>` -> `{base}/sqs/<q>`，
`http(s)://` 不受影响。无 `{host}{path}` 模板、无正则——这是旧 `WithRewrite` DSL 的极简替代。

> **为何敢重新引入「重定向」（旧 rewrite 被删，这里又回来）？** 旧 rewrite 的问题是**模板 DSL 心智负担**，
> 不是「重定向」概念本身。`WithLocalRedirect(base)` 只有一个参数、规则固定、与 mock 路由约定一一对应，
> 是 URL-scheme 寻址下唯一的本地切换点，单一真相、零模板。

### 6.2 mock 方案（单测不碰网络）

- **HTTP**：`httptest.NewServer` 起测试 server，把它的 URL 直接作为 `Do` 的 target。零特殊处理。
- **Lambda / SQS**：`WithLambdaAPI(fake)` / `WithSQSAPI(fake)` 注入实现接口的 fake，完全不起 AWS、不连 LocalStack。
- **端到端**：`WithAWSEndpoint("http://localhost:4566")` 指向 LocalStack；或 `WithLocalRedirect` + `cmd/aws-ease-mock`。

---

## 7. 包 / 文件布局 + 删除清单

### 7.1 最终布局（单包，扁平）

```
awsease.go        // package doc + Version + Backend 常量
client.go         // Client、New、config、Option*、惰性 AWS 客户端构建、SQS QueueUrl 缓存(+RWMutex)
target.go         // parseTarget、本地重定向改写、哨兵错误（ErrBadTarget / ErrUnknownScheme）
request.go        // Request 结构 + Method 默认推导
response.go       // Response 结构、OK()、JSON()、String()
do.go             // Do / DoRequest 主入口 + 内部 doHTTP/doLambda/doSQS（同包非导出，switch backend 分发）
client_test.go / target_test.go / do_test.go / example_test.go
cmd/aws-ease-mock/main.go  // 本地 HTTP mock（去掉旧信封字段，裸 echo）
doc/plan.md       // 本文
```

> 后端执行逻辑 `doHTTP/doLambda/doSQS` 作为**同包非导出函数**内联，不再拆 `transport/*` 子包，
> 因为它们只被 `DoRequest` 调用、且需共享 `Client` 的 httpClient / aws 客户端 / 缓存。import 图零层间接。

### 7.2 逐条删除清单（大刀阔斧，不保留）

| 删除对象 | 原因 |
|----------|------|
| `transportcore/types.go`（整包） | 纯为破 import cycle 而存在的多余包；其 `Response`/`Transport` 被新 `Response` + 内联函数取代。 |
| `endpoint.go`（`type Endpoint = resolver.Endpoint`） | 纯 alias 文件，额外间接层；新设计无 `Endpoint` 概念。 |
| `transport.go`（`Response`/`Transport` alias） | 纯 alias 文件；删除。 |
| `dispatcher.go` + `dispatcher_test.go` | `Dispatcher`/`Register`/scheme map 路由内联进 `DoRequest` 的 `switch backend`；不再需要可注册 transport 抽象。 |
| `resolver/`（整包：`resolver.go`/`resolver_test.go`/`rewrite_test.go`） | URL 全量解析 + `{host}{path}` 重写 DSL 被 `parseTarget`（约 20 行）+ `WithLocalRedirect` 取代。`Endpoint`/`Rewrite`/`encodeQuery` 全删。 |
| `transport/httptrans/`（整包） | 逻辑内联为 `doHTTP`；`http.Header` 多值响应头直接用标准库，不再降级成 `map[string]string` 取首值。 |
| `transport/lambdatrans/`（整包） | 逻辑内联为 `doLambda`；**删除 `requestEnvelope{path,query,payload}` 信封**（核心修正）。 |
| `transport/sqstrans/`（整包） | 逻辑内联为 `doSQS`；属性改独立 `Attributes` 字段；GroupID+DedupID 双字段。 |
| `Client.Call/Invoke/Send/Post/Get`（旧 `client.go` 88–118 行） | **别名地狱**：四个同义方法 + `Get` 每次新建 transport。全删，收成 `Do`/`DoRequest`。 |
| 旧 `WithRewrite` / `WithHTTPTransportOptions` / `WithLambdaTransportOptions` / `WithSQSTransportOptions` | 模板 DSL + 洋葱式子包 option 转发，全删。 |
| 旧 `transportcore.Response{StatusCode,Body,Headers map[string]string}` | 被新 `Response`（`http.Header` 多值 + `FuncError`/`MessageID`/`Backend` 专属字段）取代。 |

### 7.3 保留 / 改造

- `cmd/aws-ease-mock`：**保留并改造**——去掉旧 `response` 信封里的耦合字段，保持 `/lambda/`、`/sqs/`
  两个 echo 路由（与 `WithLocalRedirect` 约定对齐）。
- `Version` 常量：保留，升 `0.2.0`。
- `awsease.go` package doc：重写为新心智模型（地址 + Body -> Response）。

---

## 8. 最小使用示例

```go
// ── 构造：无参即可用；用 AWS 时注入一次 config ──
c := awsease.New(awsease.WithAWSConfig(cfg))
ctx := context.Background()
```

```go
// ── 1) HTTP：请求/响应（path、query 都在地址里）──
resp, err := c.Do(ctx, "https://api.internal/v1/users/42", nil) // Body 空 -> GET
if err != nil { return err }
if !resp.OK() { return fmt.Errorf("http %d: %s", resp.Status, resp) }
var u User
_ = resp.JSON(&u)
```

```go
// ── 2) Lambda：同步 Invoke，body 原样即 payload（无隐藏信封）──
resp, err := c.Do(ctx, "lambda://order-create", []byte(`{"sku":"A1","qty":2}`))
if err != nil { return err }                 // err 仅表示传输层失败
if resp.FuncError != "" {                    // 函数内部抛错，诚实暴露（不是假 502，不是 err）
    return fmt.Errorf("lambda %s: %s", resp.FuncError, resp)
}
fmt.Println(resp.String())                   // Lambda 返回的 payload 原样
```

```go
// ── 3) SQS：推送，返回 MessageID；Status/Body/Header 诚实为零值 ──
resp, err := c.DoRequest(ctx, awsease.Request{
    Target:     "sqs://order-events.fifo",
    Body:       []byte(`{"event":"created","id":7}`),
    GroupID:    "orders", DedupID: "order-7",
    Attributes: map[string]string{"type": "order"},
})
if err != nil { return err }
fmt.Println(resp.Backend, resp.MessageID)    // sqs e5f6...   （resp.Status==0, resp.Body==nil）
```

```go
// ── 4) 即发即忘（异步 Lambda）──
resp, _ := c.DoRequest(ctx, awsease.Request{Target: "lambda://audit-logger", Body: payload, Async: true})
// resp.Async==true, resp.Body==nil, resp.OK()==true（仅表示已投递，不代表函数已执行成功）
```

```go
// ── 5) 本地联调：业务代码一字不改，只在构造时加一行 ──
c := awsease.New(awsease.WithLocalRedirect("http://localhost:8080"))
c.Do(ctx, "lambda://order-create", body) // 实际打到 http://localhost:8080/lambda/order-create（见 mock）
```

---

## 9. 错误处理与 context / 超时约定

### 9.1 两层错误（地道 Go：error 表示「调用本身失败」）

| 层 | 触发 | 表现 |
|----|------|------|
| **传输层失败** | 地址非法 / 未知 scheme / 网络错误 / AWS 凭证或 SDK 报错 / 超时 / context 取消 / SQS body 非 UTF-8 | `Do` 返回 `err != nil`，**`resp == nil`**。 |
| **业务层失败** | HTTP 非 2xx；Lambda `FuncError` 非空 | `err == nil`，`resp != nil`，`resp.OK() == false`。 |

- 哨兵错误：`ErrBadTarget`、`ErrUnknownScheme`，调用方可 `errors.Is` 判别。
- **`err != nil` 时 `resp` 恒为 nil**（不返回半成品），消除「err 非 nil 时 resp 还能不能用」的歧义。
- 调用方标准三段式：`if err != nil {…}` → `if !resp.OK() {…}` →（需细分）`switch resp.Backend`。
- 因 scheme 显式声明后端，**不存在「错配后端却静默全绿」的风险**，故无需 `Expect` 断言（旧路由表方案才需要）。

### 9.2 context / 超时（唯一真相来源 = context）

- **统一以 `context.WithTimeout` 实现单次超时**，单一机制，避免多个超时源打架。
- `Do` 内部对传入 `ctx` 套一层 `context.WithTimeout(ctx, defaultTimeout)`；调用方若已设更短 deadline，context 自动取更早者。
- 默认 30s，由 `WithTimeout(d)` 调整；要更细控直接传 `context.WithTimeout` 的 ctx。
- **`WithHTTPClient` 注入的 `*http.Client.Timeout` 应留零**（会与 context 超时打架）——文档明确要求。
- `ctx` 取消 / deadline 命中 -> `Do` 返回包装了 `context.Canceled`/`DeadlineExceeded` 的 error。

### 9.3 并发安全

- `Client` 并发安全。SQS 队列名 -> QueueUrl 的缓存是 `map[string]string` + `sync.RWMutex`（或 `sync.Map`），**显式加锁**。
- 惰性 `aws.Config` / lambda / sqs 客户端构建用 `sync.Once`，保证只加载一次且并发安全。
- `go test -race` 必须通过（见验收）。

---

## 10. 分阶段实施任务清单

> 每个任务含**目标 / 交付物 / 验收**。建议按序执行；T01–T03 是基础，T04–T06 是三后端，T07+ 收尾。

### ✅ ~~T01 — 清空旧实现 + 搭单包骨架~~（已完成）
- 目标：删除全部旧文件，建立扁平单包结构。
- 交付物：删除 `transportcore/`、`resolver/`、`transport/`（三子包）、`dispatcher.go`(+test)、
  `endpoint.go`、`transport.go`、旧 `client.go`/`example_test.go`；新建 `client.go`/`target.go`/`request.go`/
  `response.go`/`do.go`；改写 `awsease.go`（package doc + `Version="0.2.0"` + `Backend` 常量）。
- 验收：`go build ./...` 通过（允许空实现 stub）；`go vet ./...` 无残留死引用。

### ✅ ~~T02 — 地址解析 parseTarget + 本地重定向~~（已完成）
- 目标：实现 `backend://target` 解析与 `WithLocalRedirect` 改写。
- 交付物：`target.go` 内 `parseTarget(s) (Backend, addr string, err error)`、重定向改写函数、两个哨兵错误。
- 验收：表驱动单测覆盖三 scheme（含 `sqs://https://…` 嵌套 URL、HTTP 保留整串、lambda 含 `/` 报 `ErrBadTarget`、
  未知 scheme 报 `ErrUnknownScheme`、空串报错）；重定向把 `lambda://fn`/`sqs://q` 正确改写为 `{base}/lambda/fn`、`{base}/sqs/q`。

### ✅ ~~T03 — Request / Response / Client 骨架 + options~~（已完成）
- 目标：定义统一请求/响应与构造。
- 交付物：`request.go`（含 Method 默认推导）、`response.go`（`OK()`/`JSON()`/`String()`，按第 4 章填充表）、
  `client.go`（`Client`/`config`/`New`/七个 `With*` Option/惰性 AWS 构建 + `sync.Once` + QueueUrl 缓存 `sync.RWMutex`）。
- 验收：`OK()` 对四种场景（HTTP 2xx/非2xx、Lambda FuncError 空/非空、SQS 有/无 MessageID、Async）
  用表驱动测试逐一断言；`New()` 无参且只调 HTTP 时不触发任何 AWS 凭证加载。

### T04 — doHTTP
- 目标：HTTP 后端执行。
- 交付物：`do.go` 中 `doHTTP`：target 整串作 URL、`Method` 默认推导、`Header` 写入、读 `http.Header` 多值响应头、
  填 `Status/Body/Header/Requested`。
- 验收：`httptest.NewServer` 端到端测 GET/POST、自定义 Header、非 2xx 时 `err==nil && OK()==false`、多值响应头不丢失。

### T05 — doLambda（去信封 + 诚实 FuncError + Async）
- 目标：Lambda 后端执行。
- 交付物：`doLambda`：`Body` 原样作 `Payload`（**无信封**）、`Async` -> `InvocationType=Event`、同步填 `Body`、
  `FunctionError` -> `FuncError`（**不伪造 Status，不返回 transport error**）、填 `Async/Requested`。
- 验收：fake `LambdaAPI` 断言 `InvokeInput.Payload == Request.Body`（逐字节，证明无信封）；
  函数错误场景 `err==nil && FuncError!="" && Status==0 && OK()==false`；异步场景 `Async==true && Body==nil && OK()==true`。

### T06 — doSQS（独立 Attributes + GroupID+DedupID + UTF-8 校验 + 缓存解析）
- 目标：SQS 后端执行。
- 交付物：`doSQS`：addr 为 URL 直用 / 为名走 `GetQueueUrl` 带锁缓存、`Body` UTF-8 校验、
  `Attributes` -> String `MessageAttributes`、`GroupID`/`DedupID`、填 `MessageID/Backend/Requested`（`Status==0`、`Body==nil`）。
- 验收：fake `SQSAPI` 断言 GroupID+DedupID 正确传递、属性键名不被改写、非 UTF-8 body 返回明确 error、
  队列名只解析一次（缓存命中）；`Response.Status==0 && Body==nil && MessageID!=""`。

### T07 — Do / DoRequest 编排 + 重定向 + 超时
- 目标：把三后端串成入口。
- 交付物：`Do`（= `DoRequest(Request{Target,Body})`）与 `DoRequest`：`parseTarget` -> 可选重定向 -> `switch backend`
  分发、统一 `context.WithTimeout`、`err!=nil 时 resp==nil` 约定。
- 验收：地址非法/未知 scheme 返回对应哨兵错误（`errors.Is` 通过）；超时（context 优先于 `WithTimeout` 默认）表驱动验证；
  `WithLocalRedirect` 下 lambda://、sqs:// 走 HTTP path 验证。

### T08 — mock 命令改造
- 目标：本地 mock 去信封。
- 交付物：改写 `cmd/aws-ease-mock/main.go`：保留 `/lambda/`、`/sqs/` 路由，echo 体去掉旧 `requestEnvelope` 耦合字段，
  裸回 method/path/body。
- 验收：`main_test.go` 验证两路由 echo 正确；配合 `WithLocalRedirect` 的端到端冒烟通过。

### T09 — 示例、文档、CI
- 目标：可发布质量。
- 交付物：`example_test.go`（第 8 章示例，`go test` 可跑）；`awsease.go` package doc 完善；README/godoc 对齐本规格；
  `doc/TODO.md` 标记为被本 `plan.md` 取代。
- 验收：`go test ./... -race` 全绿；`go vet ./...` 干净；`go doc ./...` 输出与本规格一致。

---

## 11. 不做什么 / 非目标

1. **不做命名路由表 / 逻辑名寻址。** 寻址就是直写 `backend://target`，零注册、可调任意目标（用户拍板）。
2. **不为 SQS 拆 `Ack` 独立返回类型。** 统一接口承诺 > 类型洁癖；诚实靠专属字段+OK()，非靠拆类型。
3. **不伪造 HTTP 状态码。** Lambda/SQS 的 `Status` 恒 0；不造 502/202。
4. **不保留 Lambda 信封** `{path,query,payload}`。`Body` 即 payload，所见即所得。
5. **不保留 `{host}{path}` 重写模板 DSL 与 `WithRewrite`。** 本地切换用极简 `WithLocalRedirect(base)` 或直接换地址串。
6. **不保留别名方法**（`Call/Invoke/Send/Post/Get`）与 `Get` 每次新建 transport 的反模式。收成 `Do`/`DoRequest`。
7. **不引入 per-call functional option**（`...CallOption`）。请求内容用 `Request` struct literal。
8. **不做后端断言 `Expect`。** scheme 显式声明后端，错配静默失败从根上不存在，无需断言。
9. **不支持 SQS Number/Binary 类型 MessageAttributes**（仅 String）。需要时后续版本扩展。
10. **不支持非 UTF-8 的 SQS body**（AWS 硬限制），实现层显式校验并报明确错误，不假装裸字节透传。
11. **不做自动重试 / 熔断 / 限流 / 批量 SendMessageBatch / SQS 接收消费端**。本库只负责「统一发起一次调用」；
    重试等横切关注点交给调用方在 `Do` 之上封装（统一返回类型正是为此预留）。
12. **不做异步 Lambda 的结果回收**。`Async` 即 fire-and-forget，诚实标注不等函数结果。
```
