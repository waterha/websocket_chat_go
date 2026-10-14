# 2026-10-14 纯结构性重构记录

## 范围与约束

基线为 `6a5fe0c`。本次仅移动声明、函数和调整导入，不修改接口、签名、返回值、错误类型、日志、配置、业务分支、默认值、SQL、执行顺序或依赖。

Go 拆分均留在原 package 中；没有增加中间接口、RPC 服务、数据库迁移或新的依赖。HTTP 路由注册和注册顺序仍在 `internal/api/server.go`。WebSocket 生命周期、Hub、outbox 与补偿任务没有改动。

## 分批变更

所有路径相对仓库根目录。

### 第一批：模型与聊天逻辑（7231360）

原 `backend/internal/model/types.go` 的声明按数据职责移动，原文件删除：

- `user.go`：用户。
- `message.go`：消息与发送输入。
- `conversation.go`：成员、会话与创建输入。
- `receipt.go`：已读、设备接收游标与投递状态。
- `realtime.go`：同步分页与事件信封。
- `error.go`：错误类型与原有构造函数。

原 `backend/internal/logic/chat.go` 保留 Chat、常量、消息扫描与 outbox 写入；新增同目录文件：

- `chat_message.go`：发送验证与消息发送。
- `chat_conversation.go`：会话序号、会话列表与创建。
- `chat_history.go`：历史查询与顺序同步。
- `chat_receipt.go`：已读、收件确认、设备校验与游标。
- `chat_delivery.go`：发送者查询成员投递状态。

这样可以按用例定位实现，不改变 Chat 的调用方式或事务边界。

### 第二批：HTTP API（17dea00）

`backend/internal/api/server.go` 保留类型声明和路由组合；新增同目录文件：

- `response.go`：响应编码、错误映射与请求解码。
- `middleware.go`：CORS、超时和身份认证。
- `auth.go`：会话 cookie、注册、登录与退出。
- `users.go`：健康检查与用户列表。
- `conversations.go`：会话列表与创建。
- `messages.go`：整数参数、历史、同步与发送。
- `receipts.go`：已读、收件确认、投递状态与设备游标。

拆分协议入口与处理职责，保留所有处理函数及其内部调用顺序。

### 第三批：压测工具（a553656）

`backend/cmd/loadtest/client.go` 保留客户端状态；新增同目录文件：

- `client_socket.go`：连接、帧读取、发送与同步提示信号。
- `client_sync.go`：顺序补拉、确认与接收循环。
- `client_http.go`：客户端 HTTP 请求及管理请求。
- `runner.go`：暂停辅助函数与压测运行流程。
- `server_metrics.go`：核验数据结构、数据库核验与资源采样。

`server.go` 保留完整的隔离服务启动及关闭流程。没有拆解其内部 fixture 创建、defer 或 goroutine，以免改变资源清理与执行时序。

### 第四批：前端请求边界（c2825f0）

`web/src/main.js` 中独立的 `api(path, options = {})` 函数移至 `web/src/api.js`，原入口通过具名导入调用。函数体、默认值、请求选项、异常类型及传播方式保持原样。

### 第五批：验证记录

增加本记录，并在 `backend/README.md` 增加结构索引链接，不改业务说明。

## 等价性验证

- 逐批检查迁移函数；最终与基线比较全部 73 个迁移范围内的 Go 声明，签名、结构字段、函数体和常量文本一致，仅统一换行用于比较。
- 前端入口除导入与移走原函数外，剩余文本一致；模板、setup 闭包及生命周期未改动。
- 在同一套隔离 PostgreSQL/Redis 上，分别运行基线与重构版本的 `go test ./... -count=1 -json`。43 条 pass/skip 结果（包含包与子测试）匹配；5 个 skip 是原本没有测试文件的包，数据库集成测试没有跳过。
- 集成测试包含千人成员投递恢复、设备确认与已读分离、持久化顺序、迁移兼容、真实 WebSocket 路由、重连及退出。
- 基线及重构版本的 6 个前端 realtime 测试均通过。
- 用临时校验程序对比请求函数的 8 组输入：成功、空数据、覆盖请求选项、业务错误、默认错误文案、空错误体、JSON 解码失败及网络失败；返回值、异常类型/文案/status、请求参数与调用顺序一致。没有新增测试文件或依赖。
- 重构版本通过 `go vet ./...`、`go build ./...`、`npm run build` 和 `git diff --check`。

这是代码等价核对与既有测试的验证记录，不是对所有输入及网络调度的穷举证明。本次没有重跑独立进程的 1000 条真实 WebSocket 性能压测，也没有修改已有压测结论。

## 未处理清单

1. **前端闭包与模板保留原样**：`web/src/main.js` 的 setup 内共享状态、异步任务、重连与销毁流程没有继续拆分；模板字符串未迁移，避免引入闭包、求值及编译差异。
2. **WebSocket 与启动函数保留原样**：`backend/internal/api/websocket.go`、压测 `serve()` 和 `load()` 的内部流程未拆解；关闭、取消、等待、defer 及 goroutine 时序保持不变。
3. **存储受限时的异常路径未改**：前端 setup 仍直接读写 sessionStorage，没有新增存储不可用时的降级处理，因为会改变异常与执行路径。
4. **非 JSON HTTP 响应仍按原方式失败**：请求工具仍直接调用 response.json()，未新增响应格式容错、重试或错误包装；原异常传播保留。
5. **现有性能与部署边界未改**：历史仍使用内存缓存及原分页恢复方式；Hub、扫描批次、锁、SQL 和单实例投递模型不变；没有增加 IndexedDB、多实例分发或新的容量承诺。
6. **测试期间的 Redis SDK 提示未处理**：一次完整测试输出出现关闭连接时的 `push: error processing pending notifications before writing pipeline` 提示；测试仍通过。没有诊断或修改依赖、关闭顺序和日志，仅记录观察。
7. **安全与治理缺口不在本次范围**：现有文档列出的限流、CSRF、审计及保留策略未实施；本次不借重构修复或增加这些功能。
