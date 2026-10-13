# API 参考

HTTP 前缀 `/api/v1`；成功 `{"data":...}`，失败 `{"error":{"code":"...","message":"..."}}`。受保护接口使用 `session` HttpOnly Cookie。JSON 使用 `snake_case`；同一 Cookie 的会话主体由服务端确定，客户端不能指定发送者。请求体不超过 64 KiB；请求的 `Origin` 必须匹配 `FRONTEND_ORIGIN`（CLI 请求可以省略）。

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/auth/register` | `{"email":"...","name":"...","password":"至少八位"}`；注册并登录 |
| POST | `/auth/login` | `{"email":"...","password":"..."}` |
| POST | `/auth/logout` | 当前会话注销并断开其 WebSocket |
| GET | `/me` | 当前用户 |
| GET | `/users` | 用户列表（当前最多 100 条） |
| GET | `/conversations` | 当前用户的会话、成员与已读游标 |
| POST | `/conversations` | `{"kind":"direct","member_ids":["另一用户ID"]}`；群聊用 `group`、`name` 和最多 49 个其他成员 |
| POST | `/conversations/{id}/messages` | `{"client_msg_id":"客户端稳定ID","content":"你好"}`；新消息 201，幂等重复 200 |
| GET | `/conversations/{id}/messages?before_seq=100&limit=30` | 从 `before_seq` 之前倒序取页，返回按 `seq` 正序；默认最新 30，最多 100 |
| GET | `/conversations/{id}/sync?after_seq=0&limit=100` | 返回 `messages,last_seq,next_seq,has_more`；按 `seq` 正序取 `after_seq` 之后最多 100 条；下一页用 `next_seq` |
| POST | `/conversations/{id}/read` | `{"seq":42}`；更新成员已读，返回 `conversation_id,user_id,seq,read_at` |
| POST | `/conversations/{id}/received` | `{"device_id":"设备ID","seq":42}`；更新设备接收游标，与已读无关 |
| GET | `/devices/{device_id}/cursors` | 当前设备对每个会话的 `received_seq` |

`client_msg_id` 长度 8–100，同一发送者全局唯一；重复提交相同会话及内容返回原消息，内容或会话不同返回 409。`seq` 在单个会话内连续；未提交的事务不会消耗序号。游标只前进；超过会话最新序号会返回 400。所有历史与同步接口都检查会话成员资格。`received_seq` 表示客户端已处理到该位置，并**不是**网络投递确认或用户已读。

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
