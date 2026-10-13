# 真实 WebSocket 压测

此工具运行两个独立进程：测试服务使用项目原有的 `api.Server`、`svc.ServiceContext`、Hub、outbox 与接收补偿；负载进程为每个成员建立独立 TCP/WebSocket 连接，并完成消息发送、HTTP 顺序补拉与接收确认。不是进程内 Hub 模拟测试。

## 前置条件

- Go 1.26 与可访问的独立 PostgreSQL、Redis。
- 不要指向生产依赖。工具创建随机 `im_load_*` schema，正常结束后删除该 schema，不修改现有业务表。
- 测试账号和一小时有效的会话直接种入测试 schema，排除注册密码哈希耗时。每次握手、HTTP 请求和心跳仍执行真实认证。
- 服务只绑定本机回环地址 `127.0.0.1:18080`，数据库池保持当前服务的 16 个连接、心跳 20 秒、慢队列 128。
- 账号 Cookie 只写入临时凭据文件，正常退出后清理；不要提交该文件。异常强杀可能留下测试 schema，应检查并人工清理。
- 性能运行必须独占测试依赖，禁止同时执行集成测试、构建或其他负载；否则只能记录为混合负载，不作为独立容量结论。

## Windows 一键执行

在项目根目录运行：

```powershell
$env:IM_TEST_DATABASE_URL='postgres://im_test:your_test_password@127.0.0.1:55433/postgres?sslmode=disable'
$env:IM_TEST_REDIS_ADDR='127.0.0.1:56379'
./scripts/loadtest.ps1
```

默认建立 1000 个连接，空闲保持 30 秒，随后分别以 1、5、10 条群消息/秒发送 20 秒，最后最多等待 60 秒补齐。每条消息的文本为 128 字节，面向全群 1000 人；共 320 条消息、320000 次成员接收。可使用 `-Members`、`-Rates`、`-Idle`、`-Duration`、`-Drain`、`-OutputDir` 调整。更长的持续测试示例：

WebSocket 握手和负载端新建 TCP 连接各自最多并行 50 个，避免在 Windows 回环接口瞬间发起上千次握手。限制只覆盖连接建立，不限制已建立连接数量、HTTP 在途请求或消息发送速率；建连等待仍计入请求延迟。这不是“1000 个用户同时发起连接”的登录风暴测试。

```powershell
./scripts/loadtest.ps1 -Rates '1,5,10' -Idle '60s' -Duration '120s' -Drain '120s'
```

脚本自动编译并启动两个独立进程，输出目录默认位于系统临时目录，保留 `result.json`、`server-metrics.jsonl`、`process-metrics.csv`、`environment.json` 和运行日志。CPU 百分比按全部逻辑处理器归一化，100% 表示占满整台机器的逻辑处理器预算；第一次样本无差分基线，CPU 值为 0。资源采样包含连接建立阶段，分析分档时应按阶段时间筛选。

## 其他系统

在 `backend` 目录编译 `go build -o /tmp/im-loadtest ./cmd/loadtest`。设置相同环境变量后，在两个终端先后运行（临时路径按平台替换）：

```text
/tmp/im-loadtest -mode server -fixture /tmp/im-credentials.json -metrics /tmp/im-server.jsonl
/tmp/im-loadtest -mode client -fixture /tmp/im-credentials.json -output /tmp/im-result.json
```

压测结束后客户端会请求测试服务正常退出；采集 CPU/进程驻留内存需要平台工具，Go 堆内存、连接数和数据库池指标由服务端自行采集。

## 指标与验收

- `send_ack_latency`：负载进程开始发送到收到持久化 ACK。
- `push_frame_latency`：开始发送到收到原始 WebSocket 推送帧；只代表推送到达，不代表补拉处理完成。
- `receive_latency`：开始发送到接收客户端按连续序号处理 HTTP 补拉结果，包含消息排队与补拉时间；接收确认落库另由最后的数据库核对证明。
- 各阶段 P50/P95/P99/Max 用原始样本的 nearest-rank 百分位计算，非平均值；接收延迟按消息所属发送阶段归类，晚到消息也计入。
- `passed` 要求全部连接已建立、最终仍在线、零非预期断连/握手/HTTP/发送/协议错误、所有发送 ACK 与数据库消息数一致、每条消息由每位成员连续处理、每位成员接收游标与最新序号一致、outbox 无积压、未改变已读。
- `passed` 是可靠性验收，不包含约定的实时延迟 SLA；高延迟即使最终补齐也应单独评估。
- 错误样本带发生时间；补拉校验错误单独记录为 `sync_errors`。最终核验失败也保留 `result.json`，不得把“最终补齐但有请求失败”写成零错误通过。

负载端同时维持 WS 读取、按提示补拉、每页接收确认和每 5 秒的设备游标对账，复用本项目的核心客户端协议。它不是 1000 个 Vue 浏览器：不包含 DOM 渲染、已读广播、每分钟全量成员元数据刷新、TLS、代理或移动网络。周期对账可以合并积压消息，消息数不能直接乘二当成实际 HTTP 请求数。

默认测试约两分钟，只能判断当前设备和当前流量下的行为；长时间稳定性、生产网络、磁盘保留增长、大消息、多群和音视频需要独立场景。结果应同时公开本机环境、目标发送速率、实际 ACK 数、延迟与可靠性，不把“支持千人群”写成未经验证的生产容量承诺。
