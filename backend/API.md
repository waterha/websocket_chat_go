# 前端对接契约建议（未实现）

这里提供字段与接口建议，方便你写 Go 后端。当前前端已包含邮箱密码注册/登录的本地演示，但尚未请求真实 API；下列路径全部需要你实现。

## 基本约定

- API 前缀 `/api/v1`，实时连接 `/ws`。
- JSON 字段用 `snake_case`，ID 用字符串，时间用 RFC3339 UTC。
- 认证推荐安全的 HttpOnly 会话 Cookie。跨站时需要明确 SameSite、CORS、CSRF 与 WebSocket Origin 校验，不在 URL 中传长期 token。
- 服务端从认证身份获取 `sender_id`，不信任客户端自报发送者。
- 成功响应 `{ "data": ... }`；错误 `{ "error": { "code": "...", "message": "..." } }`。
- 历史查询必须验证当前用户是成员；分页参数和消息长度均需服务端限制。

## HTTP 路径

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| POST | `/api/v1/auth/register` | 邮箱、密码和昵称注册 |
| POST | `/api/v1/auth/login` | 邮箱密码登录并创建会话 |
| POST | `/api/v1/auth/logout` | 销毁会话 |
| GET / PATCH | `/api/v1/me` | 读取/编辑个人资料 |
| GET | `/api/v1/contacts` | 好友列表 |
| GET | `/api/v1/conversations` | 私聊与群聊列表 |
| POST | `/api/v1/conversations` | 创建私聊/群聊，校验好友与成员资格 |
| PATCH | `/api/v1/conversations/{id}/preferences` | 当前用户置顶和静音偏好 |
| GET | `/api/v1/conversations/{id}/messages?before_seq=200&limit=30` | 游标历史，按 seq 有序返回 |
| POST | `/api/v1/conversations/{id}/messages` | 可选 HTTP 消息发送入口，与 WebSocket 共用幂等逻辑 |
| POST | `/api/v1/conversations/{id}/read` | 更新当前用户 `last_read_seq` |
| POST | `/api/v1/uploads` | 上传附件，返回服务端附件 ID |

删除当前用户可见历史与全群撤回不是同一操作。当前“清空本地演示记录”只操作浏览器，不应直接映射为删除 PostgreSQL 消息。


## 认证接口建议

### `POST /api/v1/auth/register`

请求：

```json
{
  "email": "user@example.com",
  "password": "至少 8 位的密码",
  "name": "用户昵称"
}
```

成功响应建议返回当前用户和已建立的 HttpOnly 会话 Cookie：

```json
{
  "data": {
    "user": {
      "id": "user-id",
      "email": "user@example.com",
      "name": "用户昵称"
    }
  }
}
```

### `POST /api/v1/auth/login`

请求：

```json
{
  "email": "user@example.com",
  "password": "用户密码"
}
```

密码必须在服务端使用 Argon2id 或 bcrypt 校验，禁止保存明文密码；登录失败时不要区分“邮箱不存在”和“密码错误”。

### `POST /api/v1/auth/logout`

服务端销毁当前会话并清除 Cookie，成功返回 `{ "data": null }`。

注册和登录接口都应限制请求频率，并对邮箱格式、密码长度、昵称长度和重复邮箱做服务端校验。前端本地演示中的 SHA-256 只用于模拟状态，不可替代服务端密码哈希。
## 示例消息结构

```json
{
  "id": "message-id",
  "conversation_id": "conversation-id",
  "sender_id": "user-id",
  "client_msg_id": "client-uuid",
  "seq": 201,
  "type": "text",
  "content": "你好，周末见。",
  "attachment_id": null,
  "created_at": "2026-10-08T08:30:00Z",
  "recalled_at": null
}
```

正文和附件信息分开，上传要验证大小、实际内容类型与访问权限；不要只检查文件扩展名。会话成员读取游标独立保存，不给群消息设置一个全局“已读”布尔值。

## WebSocket 事件建议

```json
{
  "event": "message.send",
  "request_id": "request-uuid",
  "data": {
    "conversation_id": "conversation-id",
    "client_msg_id": "client-uuid",
    "type": "text",
    "content": "你好，周末见。"
  }
}
```

服务端落库成功后返回 `message.ack`，携带同一个 `request_id`、`client_msg_id` 与权威消息信息。广播使用 `message.created`；读取更新使用 `conversation.read`；非法请求使用 `error`。断线后依靠已确认游标补拉消息，而不是仅靠实时广播。

为 `(sender_id, client_msg_id)` 建立幂等约束，为 `(conversation_id, seq)` 建立排序约束。具体 seq 分配、事务、投递策略由你决定，避免先 ACK 再落库。
