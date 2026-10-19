# websocket_chat_go

Go + PostgreSQL + Redis + Vue 的开源即时通讯项目。当前版本聚焦**可运行的单体 IM 基础闭环**：账号、单聊/群聊、WebSocket、离线存储、消息漫游、接收游标与已读回执。目标是逐步发展为团队沟通平台；组织架构等企业功能尚未实现。

## 已实现

- HTTP API 与 WebSocket 共用业务逻辑；按用户/设备管理长连接，心跳、同设备重连替换、慢连接断开，最多 8 个设备/用户。
- 单聊唯一会话、最多 1000 人群聊（含创建者）；批量写入成员，按会话成员向所有在线设备推送，非成员不能读写该会话。
- 私聊从联系人直接进入；若双方尚无会话则自动创建。群聊可以只有创建者，之后由群成员按需邀请其他用户加入，并在群内保留“邀请人邀请成员进入群聊”的系统提示。群聊标题栏可打开完整成员列表，按昵称或账号搜索成员，并标注本人。
- 前端侧边栏分为“当前、好友、群聊”：当前只显示有消息的最近会话，好友显示已添加用户，群聊显示当前用户加入的全部群聊；可通过“添加”发送好友申请，并在“申请”中同意或拒绝。三个列表都可按昵称、账号或群名称即时筛选。
- 设置中可以查看并修改个人用户名和用户账号。
- 聊天标题栏支持搜索当前会话的完整历史消息：关键词不区分大小写、匹配结果高亮、分页加载，点击结果可定位原消息；仅会话成员可搜索。
- 消息、会话序号和待推送事件在 PostgreSQL 同一事务提交；`(sender_id, client_msg_id)` 幂等。每个会话内 `seq` 连续递增，不承诺跨会话全局顺序。
- 不在线的消息仍持久化；`/sync` 按升序分批补拉并返回游标。推送是低延迟提示，断线、慢消费或进程异常时以数据库补拉为准。
- 每位成员的已读位置与每台设备的接收位置分离，位置只前进不后退。接收不等于已读；已读变更推送给会话成员。
- 发送者可按消息查看其他成员的接收确认人数，明确区分“服务器已保存”“成员设备声明已接收”“用户已读”；查询失败或未确认不冒充成功。
- 服务端每 5 秒按在线设备接收游标与会话最新序号对账；未确认收齐时持续发送定向补拉提示，不依赖 outbox 已发布状态或已读状态。客户端连续处理消息后才确认，确认失败会重试；离线成员重新上线后补齐。
- Vue 客户端支持重连、定期补拉、多页漫游、已读状态与同 ID 安全重试；旧 `frontend/` 是不连服务的视觉演示。

## 本地运行

需要 Go 1.26、Node.js 22+、PostgreSQL 17 和 Redis 7。可以使用 Docker Compose 启动依赖：

Windows 本机也可以使用一键启动脚本（默认识别当前项目使用的便携版 PostgreSQL 和 Redis）：

```powershell
Set-Location C:\Sofware\Go_proj\websocket_chat_go
.\scripts\start.ps1
```

脚本会自动跳过已经运行的服务，并在新窗口启动后端和前端。若 PowerShell 禁止执行脚本，可使用：

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\start.ps1
```

如 PostgreSQL 或 Redis 安装在其他位置，可在启动前设置 `IM_POSTGRES_BIN`、`IM_POSTGRES_DATA`、`IM_REDIS_DIR`。

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

接口示例见 [backend/API.md](backend/API.md)，运行和架构说明见 [backend/README.md](backend/README.md)，数据表见 [database/README.md](database/README.md)。历史消息存储的技术比较、保留 PostgreSQL 的依据及本轮索引优化验证见 [存储调研](docs/research/2026-10-18-message-history-storage.md)。

## 群消息防漏收

即时广播不等于接收成功。消息先持久化，WebSocket 快速推送；独立对账检查每个在线设备是否确认到最新 `seq`，缺失则反复提醒按 `/sync` 补拉。客户端只确认连续序号，重复消息不重复展示；慢连接断开后重连恢复，离线成员仍保留补拉资格。即使最后一条广播丢失、没有后续消息，对账也能发现缺口。

保证的是：消息仍保留在数据库，成员继续有访问权限，数据库/网络恢复且客户端重新上线并遵循接收协议时，最终可以补齐。无法保证永久离线或不处理补拉提示的客户端收到消息，也不保证所有人同一时刻收到。接收确认与已读完全独立；当前浏览器缓存是内存，重新加载仍从数据库恢复。1000 人是群成员上限，不是已通过生产并发压测的容量承诺。

浏览器在发送前把稳定消息 ID 与原文保存在当前标签页的 `sessionStorage`：若发送结果未知，刷新同一标签页后仍可恢复原文并用同一 ID 安全重试；同步到相同消息或收到发送 ACK 后清除记录。修改原文前须先明确放弃旧确认，放弃**不代表服务器撤回消息**。退出登录会清除记录；关闭标签页、清除浏览器存储或换设备时不保证保留待确认 ID，不应据此断言发送失败。

## 开发验证

真实 1000 条 WebSocket 长连接压测工具与复现方式见 [压测说明](docs/performance/README.md)，本机实测结果、失败轮次与延迟边界见 [2026-10-13 压测报告](docs/performance/2026-10-13-1000-websocket.md)。测试包含群消息发送、全员顺序补拉和接收确认，不把短时本机测试等同于生产容量承诺。

```powershell
Set-Location backend
go test ./...
go vet ./...
Set-Location ../web
node --test src/*.test.js
npm run build
```

集成测试需要独立 PostgreSQL 数据库和 Redis：设置 `IM_TEST_DATABASE_URL`、`IM_TEST_REDIS_ADDR` 后运行 `go test ./...`；每个测试创建自己的 schema 并清理。CI 会运行完整测试。

## 当前边界

**只能部署一个服务实例。** Redis 仅维护短期连接在线标记，**不负责跨实例广播**；持久化 outbox 与进程内分发也不代表客户端已经收到。还没有组织/租户、RBAC、文件、跨会话全局搜索、推送通知、音视频、端到端加密、审计、登录限流和完整 CSRF 防护，不应直接作为企业生产系统暴露到公网。扩容前需设计跨网关事件投递及其监控、保留策略和安全治理。

参与贡献见 [CONTRIBUTING.md](CONTRIBUTING.md)，安全反馈见 [SECURITY.md](SECURITY.md)。
