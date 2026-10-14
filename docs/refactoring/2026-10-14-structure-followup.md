# 2026-10-14 结构性重构补充记录

本轮接续第一轮，基线为 `d37d53a`。所有改动仍限于同 package 内的声明迁移及导入调整，不增加依赖、接口、逻辑分支、默认值、重试或错误处理，不修改 SQL、配置和日志文案。

## 分批结构变更

### 存储职责分离（3c99f0b）

- `backend/internal/model/store.go` 保留 Store 类型。
- 新增 `backend/internal/model/identity.go`，原样迁移 NewID 与 TokenHash。
- 新增 `backend/internal/model/migrations.go`，原样迁移 Migrate。

理由：连接持有对象、密码学标识工具与迁移流程属于不同职责。公开名称与 package 保持不变；随机字节读取、panic、哈希输出、迁移锁、事务和清理顺序都没有改动。

### WebSocket 文件分离（f697fa7）

- `backend/internal/api/websocket.go` 保留完整的 websocket 入口函数。
- 新增 `backend/internal/api/websocket_commands.go`，原样迁移 command、parseJSON 与 handleCommand。
- 新增 `backend/internal/api/websocket_writer.go`，原样迁移 presence 与 writeLoop。

理由：协议解析和业务命令路由与连接写入、心跳、在线标记属于不同职责。本轮只移动已经存在的完整函数，不提取新的子函数，不修改原有调用点；连接建立、读循环、关闭、取消、等待及 defer 的内部执行顺序均不变。

### 集成测试主题分离（1d57c2e）

- `backend/internal/api/integration_test.go` 保留 fixture、账户与状态工具以及 WebSocket/HTTP 测试辅助对象和函数。
- 新增 `integration_messaging_test.go`，原样迁移消息持久化、顺序和游标测试。
- 新增 `integration_migration_test.go`，原样迁移历史数据库迁移测试。
- 新增 `integration_websocket_test.go`，原样迁移真实 WebSocket 路由、重连、退出和严格协议校验测试。

理由：原文件 624 行，包含多个独立测试主题。拆分便于定位，但不拆测试函数内部的连续场景、不改断言、fixture、子测试或并发调度。文件命名后额外核对了 Go 工具发现顶层测试的顺序，顺序与基线一致。

### 文档记录

新增本记录并在后端说明中增加索引链接。第一轮记录作为历史记录保留；其“未改文件”的说明指第一轮，不表示本轮也完全没有移动该文件的声明。

## 验证结果

- 比较基线与当前迁移范围内全部 24 个 Go 声明：数量、名称、签名、字段、函数体一致，仅统一换行用于文本比较。
- 每批均运行隔离 PostgreSQL/Redis 集成测试；迁移、消息、WebSocket、重连与退出等原有断言全部通过。
- 本轮开始与结束在同一套隔离依赖上分别运行 `go test ./... -count=1 -json`；43 条 pass/skip 结果（包含包与子测试）一致。
- 5 条 skip 为没有测试文件的包；没有跳过任何测试用例或数据库集成测试。
- 7 个 API 顶层测试的发现顺序一致，原有子测试和并发指令未变。
- 最终通过 `go vet ./...`、`go build ./...`、6 个前端实时通信测试、`npm run build` 和 `git diff --check`。
- 测试使用隔离 schema，测试依赖在验收后停止；本轮未改变数据库迁移文件或依赖清单。

验证是源码等价核对与现有测试覆盖，不是对所有输入、运行时间、网络调度或并发交错的穷举证明。文件名和源码行号会随结构迁移改变，不承诺调试堆栈位置一致；错误类型、错误文案和显式日志保持不变。本轮没有重跑千条真实 WebSocket 性能压测，不增加容量承诺。

## 未处理清单

1. **复杂函数内部保留原样**：websocket、writeLoop、handleCommand、Migrate 及消息集成测试内部未进一步提取；原来的 defer、goroutine、事务、异常传播和执行顺序保留。
2. **前端不再拆分**：setup 闭包与模板仍保持第一轮状态；本轮没有调整任何前端源码或请求接口。
3. **没有为拆分强建新层**：model 原 package、Auth 与 Credentials、Hub 和压测启动/运行函数保持现状；未增加 repository 接口、RPC、通用工具包或转发层。
4. **测试中 Redis SDK 关闭连接提示再次出现**：最终集成测试在退出场景出现 pending notifications 与 closed network connection 提示，但用例通过。本轮基线运行未出现该提示；不能据此把不同调度下的日志输出声称为逐字相同。没有改动关闭逻辑、依赖或日志，也未诊断成因。
5. **已有功能边界不变**：存储不可用时的异常、非 JSON 响应、内存历史缓存、单实例投递及安全治理缺口仍沿用第一轮未处理清单，没有借结构整理增加容错或修复。

剩余文件主要是职责较集中模块或依赖执行时序的函数。继续为了缩短文件而拆分，会增加跨文件跳转或引入额外状态边界；本轮在以上位置停止。
