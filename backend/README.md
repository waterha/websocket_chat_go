# 后端开发说明

当前是一个进程内的单体服务，参考 go-zero 的 API/业务逻辑分层，但**未引入 go-zero RPC 框架、gRPC 或服务发现**：

```text
cmd/server → internal/api（HTTP 与 WebSocket 适配）
                   ↓
             internal/logic（Auth、Chat 用例）
                   ↓
             internal/model（DTO、PostgreSQL 与迁移）

internal/svc（依赖组合）  internal/realtime（设备连接 Hub、outbox 分发）
```

`api` 只解析身份、协议和输入；HTTP 和 WS 共用 `logic.Chat`。将来若拆分 RPC，业务用例是候选边界；现在避免引入额外的分布式部署成本。

## 消息生命周期

1. 认证发送者与会话成员，依据 `(sender_id, client_msg_id)` 处理重试。
2. 在事务中锁定会话行，分配 `next_seq`，写入消息与 outbox 事件，一起提交。ACK 仅表示持久化成功，不表示接收者已收。
3. 单实例 dispatcher 扫描未发布 outbox 并向本地 Hub 的会话成员设备推送。若发送通道已满，关闭慢连接；用户仍可补拉。
4. 独立 reconciler 每 5 秒将在线设备分批（每批 100 个）与数据库会话最新序号对账，对接收游标落后的设备发送 `message.sync_required`。只检查 `received_seq`，不检查已读；即使 outbox 已发布或推送完全没发生也能补偿。
5. 客户端收到提示或重连后按 `/sync` 的 `next_seq` 连续补拉；每页处理后上报设备接收游标，未确认会持续重试。用户在可见聊天窗口确认阅读时，再单独上报已读游标。接收确认持久化在 PostgreSQL，服务重启不会清空。

Outbox 是服务端在断点后的推送恢复机制；`published_at` 仅代表提交给本进程队列，不是网络成功投递，崩溃时可能重复推送。客户端应以数据库补拉修复遗漏并对消息去重。每个会话有序，跨会话没有全局顺序保证。Redis 只记录带 TTL 的连接在线标记，重连时不同连接采用不同 key 以避免旧连接清掉新状态。

Vue 初版只在内存保存本次会话的消息缓存，页面重载从 `seq=0` 分页恢复，不把服务端设备游标误当作本地持久缓存。聊天窗口展示最新 100 条，可逐次显示更早记录；长期使用和大历史量应增加 IndexedDB 缓存、按需漫游与保留策略。

群聊含创建者最多 1000 人，成员使用单条批量 INSERT；在线推送使用有界队列，一个慢设备只会断开自己的连接。客户端每 5 秒使用轻量游标对账，不再每条消息重复拉取千人成员列表；约每 60 秒刷新元数据，收到新会话事件立即刷新。消息提示在进行中的补拉期间到达时，提高补拉目标，避免最后一条消息因任务合并而漏收。

接收可靠性测试覆盖千人成员边界、999 个初始在线设备的完整广播、离线/慢设备恢复、丢弃全部即时广播后的补拉、确认丢失重复提示、全部 1000 成员顺序补齐、按设备停止重试、重启后游标保留，以及接收不改变已读。此测试不代表 1000 个真实 WebSocket 的吞吐/资源压测。

## 本地运行和验证

复制根目录 `.env.example` 为 `.env` 并填写密码，启动 PostgreSQL/Redis，然后在 `backend/` 运行 `go run ./cmd/server`。服务启动时检查依赖并运行 `database/migrations` 中的版本化迁移。`MIGRATIONS_DIR` 可覆盖目录。数据库 TLS 使用 pgx 支持的标准 `PGSSLMODE` 等设置。

`go test ./...` 运行单元测试；设置 `IM_TEST_DATABASE_URL` 和 `IM_TEST_REDIS_ADDR` 后会额外运行真实数据库/WebSocket 测试，使用临时 schema，**不要**指向无法创建/删除临时 schema 的生产数据库。可用 `go test -race ./...` 排查并发问题。

生产部署前需要 TLS、独立迁移账号、限流、CSRF 防护、访问审计与监控；此版本不支持多实例横向扩容。
