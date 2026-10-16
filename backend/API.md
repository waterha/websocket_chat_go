# API 参考

HTTP 前缀 `/api/v1`；成功 `{"data":...}`，失败 `{"error":{"code":"...","message":"..."}}`。受保护接口使用 `session` HttpOnly Cookie。JSON 使用 `snake_case`；同一 Cookie 的会话主体由服务端确定，客户端不能指定发送者。请求体不超过 64 KiB；请求的 `Origin` 必须匹配 `FRONTEND_ORIGIN`（CLI 请求可以省略）。

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/auth/register` | `{"email":"...","name":"...","password":"至少八位"}`；注册并登录 |
| POST | `/auth/login` | `{"email":"...","password":"..."}` |
| POST | `/auth/logout` | 当前会话注销并断开其 WebSocket |
| GET | `/me` | 当前用户 |
| PATCH | `/me` | 修改当前用户的用户名和用户账号：`{"name":"新用户名","email":"new@example.com"}`；用户 ID 不变，账号重复返回 `email_exists` |
| GET | `/users` | 用户列表（当前最多 1000 条） |
| GET | `/friends` | 当前用户已添加的好友 |
| POST | `/friends` | 添加好友：`{"user_id":"用户ID"}`；重复添加幂等 |
| GET | `/conversations` | 当前用户的会话、成员与已读游标 |
| POST | `/conversations` | 私聊由联系人点击时用 `{"kind":"direct","member_ids":["另一用户ID"]}` 自动获取或创建唯一会话；群聊用 `group`、`name` 和可选的最多 999 个其他成员，创建者可以先单独建群，含创建者最多 1000 人 |
| POST | `/conversations/{id}/members` | 群成员邀请用户：`{"member_ids":["用户ID"]}`；重复成员幂等，含现有成员最多 1000 人；新成员加入时按人写入一条可漫游的群聊系统消息 |
| POST | `/conversations/{id}/messages` | `{"client_msg_id":"客户端稳定ID","content":"你好"}`；新消息 201，幂等重复 200 |
| GET | `/conversations/{id}/messages?before_seq=100&limit=30` | 从 `before_seq` 之前倒序取页，返回按 `seq` 正序；默认最新 30，最多 100 |
| GET | `/conversations/{id}/sync?after_seq=0&limit=100` | 返回 `messages,last_seq,next_seq,has_more`；按 `seq` 正序取 `after_seq` 之后最多 100 条；下一页用 `next_seq` |
| POST | `/conversations/{id}/read` | `{"seq":42}`；更新成员已读，返回 `conversation_id,user_id,seq,read_at` |
| POST | `/conversations/{id}/received` | `{"device_id":"设备ID","seq":42}`；更新设备接收游标，与已读无关 |
| GET | `/conversations/{id}/messages/{seq}/delivery` | 仅消息发送者可查：返回其他成员中至少一台设备确认到该序号的人数、其他成员总数及查询时间；找不到或无权查看返回 404 |
| GET | `/devices/{device_id}/cursors` | 当前设备对每个会话的 `conversation_id,device_id,received_seq,last_seq`；用最新序号对账 |

`client_msg_id` 长度 8–100，同一发送者全局唯一；重复提交相同会话及内容返回原消息，内容或会话不同返回 409。`seq` 在单个会话内连续；未提交的事务不会消耗序号。游标只前进；超过会话最新序号会返回 400。所有历史与同步接口都检查会话成员资格。`received_seq` 是客户端声明已经连续处理到该位置的应用层接收确认，并**不是**服务端写入网络成功或用户已读；不得跳过缺失序号确认。

`/delivery` 示例：`{"conversation_id":"...","seq":42,"confirmed_members":3,"total_members":9,"as_of":"..."}`。统计排除发送者，同一成员多台设备只计一次；某成员任意一台设备的接收游标达到该 `seq` 就计入。它是**查询时客户端所声明的应用层接收进度**，不是设备在线、用户已读或用户实际看到了消息；未确认也不能推断一定没收到。结果只对查询时的成员集合有效，返回时间后可能变化。服务端不为恶意或错误客户端的虚假确认提供物理收包证明。

## WebSocket

`GET /ws?device_id=<设备ID>` 沿用 Cookie；必须带有准确的 `Origin`。设备 ID 为 1–100 位英文字母、数字、`_` 或 `-`；同设备重新连接会替换先前连接。收到 `connection.ready` 后再发送命令：

```json
{"event":"message.send","request_id":"request-unique-id","data":{"conversation_id":"会话ID","client_msg_id":"stable-retry-id","content":"你好"}}
```

`message.ack` 返回**已经提交到数据库**的消息及相同 `request_id`；失败返回带相同 `request_id` 的 `error`。其他命令：

| 命令 | `data` | 响应 |
| --- | --- | --- |
| `conversation.read` | `{"conversation_id":"...","seq":42}` | `conversation.read.ack` |
| `message.received` | `{"conversation_id":"...","seq":42}` | `message.received.ack`（设备取自连接） |
| `ping` | 可为 `null` | `pong` |

服务端向该会话所有在线成员设备推送 `message.created`、`conversation.created`、`conversation.read`。一次发送可能先收到推送再收到 ACK；推送可能重复或丢失。断线、超时或 ACK 丢失时，用原 `client_msg_id` 重试，再从最后**连续处理**的 `seq` 通过 `/sync` 补拉，按消息 `id/seq` 去重；不要把广播当成可靠投递。历史接口用于往前浏览，同步接口用于往后漫游。

## 接收补偿（与已读无关）

服务端每 5 秒分批检查在线设备的接收游标，低于会话最新序号时发送 `message.sync_required`，每个事件最多包含 100 个会话：

```json
{"event":"message.sync_required","data":{"cursors":[{"conversation_id":"会话ID","device_id":"desktop","received_seq":40,"last_seq":42}]}}
```

客户端从自己的本地连续游标（不能盲用服务端游标）补拉到 `last_seq`，处理后用 HTTP `/received` 或 WS `message.received` 确认。未确认的设备会在后续对账再次收到提示，确认丢失时可重复确认，不修改已读。客户端同时应定期查询 `/devices/{device_id}/cursors`，修复补拉提示自身丢失；新的实时提示到达进行中的补拉时，应提高该任务的目标序号，而不是丢掉最后一条提示。

当前 Vue 客户端每 5 秒轻量对账、约每 60 秒刷新完整会话元数据；断线重连或发现新会话时完整刷新。消息在数据库中只保存一份，1000 个成员各自补拉，不需要为每条消息复制 1000 份内容。补偿依赖数据库可用、消息仍保留以及客户端恢复连接并正确确认；不承诺永久离线设备的送达时间。
