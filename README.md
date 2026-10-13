# websocket_chat_go

Go + PostgreSQL + Redis + Vue 的开源即时通讯项目。当前版本聚焦**可运行的单体 IM 基础闭环**：账号、单聊/群聊、WebSocket、离线存储、消息漫游、接收游标与已读回执。目标是逐步发展为团队沟通平台；组织架构等企业功能尚未实现。

## 已实现

- HTTP API 与 WebSocket 共用业务逻辑；按用户/设备管理长连接，心跳、同设备重连替换、慢连接断开，最多 8 个设备/用户。
- 单聊唯一会话、最多 50 人群聊；按会话成员向所有在线设备推送，非成员不能读写该会话。
- 消息、会话序号和待推送事件在 PostgreSQL 同一事务提交；`(sender_id, client_msg_id)` 幂等。每个会话内 `seq` 连续递增，不承诺跨会话全局顺序。
- 不在线的消息仍持久化；`/sync` 按升序分批补拉并返回游标。推送是低延迟提示，断线、慢消费或进程异常时以数据库补拉为准。
- 每位成员的已读位置与每台设备的接收位置分离，位置只前进不后退。接收不等于已读；已读变更推送给会话成员。
- Vue 客户端支持重连、定期补拉、多页漫游、已读状态与同 ID 安全重试；旧 `frontend/` 是不连服务的视觉演示。

## 本地运行

需要 Go 1.26、Node.js 22+、PostgreSQL 17 和 Redis 7。可以使用 Docker Compose 启动依赖：

```powershell
Copy-Item .env.example .env
# 修改 .env 内 POSTGRES_PASSWORD
docker compose --env-file .env -f infra/compose.yaml up -d
```

在两个终端分别运行：

```powershell
Set-Location backend
go run ./cmd/server
```

```powershell
Set-Location web
npm ci
npm run dev
```

打开 `http://127.0.0.1:4173`。后端监听 `127.0.0.1:8080`，`GET /healthz` 检查依赖。服务在启动时读取根目录 `.env`，执行版本化迁移；已有最初版本的数据库也可升级。生产环境应由独立迁移任务执行 DDL，并关闭服务账号的 DDL 权限。

接口示例见 [backend/API.md](backend/API.md)，运行和架构说明见 [backend/README.md](backend/README.md)，数据表见 [database/README.md](database/README.md)。

## 开发验证

```powershell
Set-Location backend
go test ./...
go vet ./...
Set-Location ../web
node --test src/realtime.test.js
npm run build
```

集成测试需要独立 PostgreSQL 数据库和 Redis：设置 `IM_TEST_DATABASE_URL`、`IM_TEST_REDIS_ADDR` 后运行 `go test ./...`；每个测试创建自己的 schema 并清理。CI 会运行完整测试。

## 当前边界

**只能部署一个服务实例。** Redis 仅维护短期连接在线标记，**不负责跨实例广播**；持久化 outbox 与进程内分发也不代表客户端已经收到。还没有组织/租户、RBAC、文件、搜索、推送通知、音视频、端到端加密、审计、登录限流和完整 CSRF 防护，不应直接作为企业生产系统暴露到公网。扩容前需设计跨网关事件投递及其监控、保留策略和安全治理。

参与贡献见 [CONTRIBUTING.md](CONTRIBUTING.md)，安全反馈见 [SECURITY.md](SECURITY.md)。
