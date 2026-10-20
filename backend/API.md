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
| POST | `/friends` | 发送好友申请：`{"user_id":"用户ID","message":"可选申请信息"}`；同一对用户的待处理申请重复提交返回原申请 |
| GET | `/friends/requests` | 返回当前用户收到的待处理申请，以及自己发出的全部申请及状态 |
| POST | `/friends/requests/{id}/accept` | 同意收到的好友申请，建立双向好友关系 |
| POST | `/friends/requests/{id}/reject` | 拒绝收到的好友申请；发起方可在发出申请列表看到 `rejected` |
| GET | `/conversations` | 当前用户的会话、成员与已读游标 |
| POST | `/conversations` | 私聊由联系人点击时用 `{"kind":"direct","member_ids":["另一用户ID"]}` 自动获取或创建唯一会话；群聊用 `group`、`name` 和可选的最多 999 个其他成员，创建者可以先单独建群，含创建者最多 1000 人 |
| POST | `/conversations/{id}/members` | 群成员邀请用户：`{"member_ids":["用户ID"]}`；任意群成员可邀请，重复成员幂等，含现有成员最多 1000 人；新成员加入时按人写入一条可漫游的群聊系统消息 |
| DELETE | `/conversations/{id}/members` | 移除成员：`{"user_id":"用户ID"}`；群主与管理员可调用，管理员不能移除群主或其他管理员，群主不能被移除（需先转让），每次移除写入系统提示 |
| POST | `/conversations/{id}/members/leave` | 当前用户退出群聊；群主需先转让群主，否则返回 403 `owner_must_transfer` |
| POST | `/conversations/{id}/roles` | 设置成员角色：`{"user_id":"用户ID","role":"admin"}` 或 `"member"`；仅群主可调用，群主身份不能通过该接口授予 |
| POST | `/conversations/{id}/owner` | 转让群主：`{"user_id":"用户ID"}`；仅现任群主可调用，原群主变为管理员 |
| PATCH | `/conversations/{id}/announcement` | 群公告：`{"announcement":"最多 2000 字，可清空"}`；群主与管理员可调用，空字符串表示清除 |
| POST | `/conversations/{id}/messages` | `{"client_msg_id":"客户端稳定ID","content":"你好"}`；新消息 201，幂等重复 200。可带 `media_id`（附件）、`reply_to_id`（引用同会话消息）、`mentions`（提及成员 ID 数组） |
| POST | `/conversations/{id}/messages/{seq}/recall` | 撤回消息；发送者本人在 2 分钟内可撤回，群主/管理员可撤回本群任意消息；已撤回的消息内容不再返回 |
| POST | `/conversations/{id}/media?file_name=photo.png` | 上传图片或视频：请求体为文件原始字节，`Content-Length` 必填，`Content-Type` 决定类型；成功 201 返回附件元数据，见下文 |
| GET | `/media/{media_id}` | 下载附件原始字节；仅该会话成员可访问，返回 `Content-Type`、`Content-Disposition: inline`，见下文 |
| GET | `/conversations/{id}/messages?before_seq=100&limit=30` | 从 `before_seq` 之前倒序取页，返回按 `seq` 正序；默认最新 30，最多 100 |
| GET | `/conversations/{id}/messages/search?q=hello&before_seq=100&limit=30` | 仅当前会话成员可搜索，按 `seq` 倒序返回 `messages,has_more,next_before_seq`，详见下文 |
| GET | `/conversations/{id}/sync?after_seq=0&limit=100` | 返回 `messages,last_seq,next_seq,has_more`；按 `seq` 正序取 `after_seq` 之后最多 100 条；下一页用 `next_seq` |
| POST | `/conversations/{id}/read` | `{"seq":42}`；更新成员已读，返回 `conversation_id,user_id,seq,read_at` |
| POST | `/conversations/{id}/received` | `{"device_id":"设备ID","seq":42}`；更新设备接收游标，与已读无关 |
| GET | `/conversations/{id}/messages/{seq}/delivery` | 仅消息发送者可查：返回其他成员中至少一台设备确认到该序号的人数、其他成员总数及查询时间；找不到或无权查看返回 404 |
| GET | `/devices/{device_id}/cursors` | 当前设备对每个会话的 `conversation_id,device_id,received_seq,last_seq`；用最新序号对账 |

`client_msg_id` 长度 8–100，同一发送者全局唯一；重复提交相同会话及内容返回原消息，内容或会话不同返回 409。`seq` 在单个会话内连续；未提交的事务不会消耗序号。游标只前进；超过会话最新序号会返回 400。所有历史与同步接口都检查会话成员资格。`received_seq` 是客户端声明已经连续处理到该位置的应用层接收确认，并**不是**服务端写入网络成功或用户已读；不得跳过缺失序号确认。

## 会话消息搜索

`GET /api/v1/conversations/{id}/messages/search?q=hello&before_seq=100&limit=30` 使用 `session` Cookie；未登录返回 401，会话不存在或当前用户不是成员均返回 404 `not_found`。仅查询该会话数据库中的消息内容（包括系统消息），不依赖客户端已加载的历史。

- `q` 必填，先去除首尾 Unicode 空白，再验证为 1–100 个 Unicode 字符（rune）；不合法返回 400 `invalid_query`。
- 大小写不敏感，按 PostgreSQL `lower` 的数据库区域规则比较字面子串；`%`、`_`、反斜线均无通配符语义。查询值应正常 URL 编码，例如 `%` 编码为 `%25`。
- `before_seq` 可省略，默认从最新消息往前查询；提供时必须为正的 int64，按 `seq < before_seq` 排他过滤；空值、非整数、溢出或非正数返回 400 `invalid_cursor`。可以大于会话最新序号。
- `limit` 可省略，默认 30；提供时必须为 1–100 的整数，空值、非整数、溢出或越界返回 400 `invalid_limit`。
- 成功返回 `{"data":{"messages":[...],"has_more":true,"next_before_seq":42}}`。`messages` 是标准消息对象，按 `seq DESC` 排列；`has_more` 表示同一查询还有更旧的匹配消息，下一页沿用相同 `q` 并设置 `before_seq=next_before_seq`。`next_before_seq` 为本页末条消息的 `seq`，即使 `has_more=false` 也保留；空页固定为 `{"messages":[],"has_more":false,"next_before_seq":0}`，不应再翻页。

`/delivery` 示例：`{"conversation_id":"...","seq":42,"confirmed_members":3,"total_members":9,"as_of":"..."}`。统计排除发送者，同一成员多台设备只计一次；某成员任意一台设备的接收游标达到该 `seq` 就计入。它是**查询时客户端所声明的应用层接收进度**，不是设备在线、用户已读或用户实际看到了消息；未确认也不能推断一定没收到。结果只对查询时的成员集合有效，返回时间后可能变化。服务端不为恶意或错误客户端的虚假确认提供物理收包证明。

## 回复、提及与撤回

消息对象新增 `reply_to_id`、`reply`、`mentions` 与 `recalled_at`。所有读取路径（历史、同步、搜索、WebSocket 推送、ACK）都返回这些字段。

- **回复**：发送时带 `reply_to_id`。服务端校验被引用消息属于同一会话，否则 404 `not_found`；跨会话引用不会被接受。`reply` 是服务端生成的引用上下文：`{"id","seq","sender_id","kind","excerpt","recalled","media_type"}`，`excerpt` 最多 120 个字符，附件消息按类型显示。原文被撤回后 `recalled=true` 且 `excerpt` 为空。
- **提及**：`mentions` 是最多 100 个成员用户 ID；重复值被去重并按 ID 排序，非成员或空值返回 400 `invalid_mentions`。提及只是结构化标注与客户端高亮/通知依据，**不是**投递保证。
- **撤回**：`POST /conversations/{id}/messages/{seq}/recall`。发送者本人可在**发送后 2 分钟内**撤回，超时返回 403 `recall_window_expired`；群主与管理员不受该窗口限制，可撤回本群任意成员的消息（管理场景）。其他人返回 403 `forbidden`。撤回**保留消息行与会话序号**，因此历史仍然连续、`/sync` 游标不受影响；`content` 与附件元数据不再返回，客户端应显示占位文本。重复撤回幂等。撤回不删除磁盘上的附件字节。
- 幂等比较包含 `content`、`media_id`、`reply_to_id` 与 `mentions`，其中任一不同而 `client_msg_id` 相同会返回 409 `message_conflict`。
- 撤回推送 `message.recalled` 事件（载荷为撤回后的消息），与 `message.created` 一样可能重复或丢失，客户端应通过 `/sync` 兜底。

当前没有：撤回后还原、管理员撤回审计记录、提及的定向通知与免打扰规则、@所有人。



## 群组治理

群聊含创建者最多 1000 人。创建者自动成为**群主**，其余成员为**成员**；群主可把成员设为**管理员**。每个群有且只有一个群主，`conversation_members.role` 与部分唯一索引 `conversation_members_owner_idx` 共同保证这一点。私聊没有群主，且所有群治理接口对私聊返回 400 `invalid_conversation`。

| 操作 | 允许的角色 | 失败情形 |
| --- | --- | --- |
| 邀请成员 | 任意群成员 | 非成员返回 404 |
| 移除他人 | 群主、管理员 | 管理员不能移除群主或其他管理员；群主不可被移除（返回 403 `owner_must_transfer`） |
| 退群 | 任意群成员（本人） | 群主必须先转让，否则 403 `owner_must_transfer` |
| 设为/取消管理员 | 仅群主 | 不能授予群主身份，返回 400 `invalid_role` |
| 转让群主 | 仅现任群主 | 目标必须是本群成员，否则 404；转让后原群主变为管理员 |
| 编辑群公告 | 群主、管理员 | 超过 2000 字返回 400 `invalid_announcement` |

移除成员、退群、角色变更与群主转让都会写入一条 `kind='system'` 的系统提示并复用会话序号，因此和其他消息一样可以被离线成员补拉漫游。**移除成员不删除其历史消息**：`messages` 不再要求发送者当前仍是成员（发送时由应用层校验成员资格），所以群内历史仍然完整、可搜索。群公告、角色与群主写在 `conversations.announcement`、`conversations.created_by` 与 `conversation_members.role`。

`GET /conversations` 的成员对象带 `role` 字段并按「群主、管理员、其他成员」排序，会话对象带 `announcement` 与 `owner_id`。群元数据变化时向全群推送 `conversation.updated`，客户端据此刷新成员列表与公告。当前没有：自定义权限、禁言、成员邀请审批、群分组与审计日志。管理员不能移除其他管理员是刻意设计，避免管理员互相驱逐；如需该能力应由群主调整角色后操作。



## 图片与视频

消息支持 `kind` 为 `image` 或 `video` 的附件消息。附件先上传再发送：`POST /conversations/{id}/media` 只接受图片或视频，`GET /media/{media_id}` 只在调用者是所属会话成员时返回字节。**上传者与发送者必须是同一人**，且附件必须属于目标会话，因此上传凭证不能被借用。

- 支持的类型由 `Content-Type` 决定：`image/jpeg`、`image/png`、`image/gif`、`image/webp` 以及 `video/mp4`、`video/webm`、`video/ogg`；其他类型返回 415 `unsupported_media_type`。请求体是原始字节，不是 multipart，也不是 JSON。服务端**不嗅探文件内容**，声明的类型即存储与下载类型，因此任意字节都可以被标成图片；下载方应把返回内容视为不可信媒体。
- 图片上限 10 MiB，视频上限 100 MiB；超限返回 413 `media_too_large`。缺失或非正的 `Content-Length` 返回 400 `invalid_upload`；写入长度与声明不一致同样返回 400。
- 上传非成员会话返回 404 `not_found`。成功 201 返回 `{"id","conversation_id","file_name","media_type","content_type","byte_size","created_at"}`；`file_name` 已去除路径部分并截断到 255 个字符。存储键不出现在任何响应中。
- 发送附件消息：`{"client_msg_id":"...","content":"可选说明","media_id":"上传返回的 id"}`。`kind` 由附件类型推导，不读取客户端声明；`content` 可以为空字符串（最多 4000 字符）作为说明。引用不存在、不属于该会话或非本人上传的 `media_id` 返回 404；`content` 为空且没有 `media_id` 仍返回 400。重试的幂等比较同时包含 `content` 与 `media_id`，两者任一不同返回 409 `message_conflict`。
- `GET /media/{media_id}` 对非成员或不存在返回 404 `not_found`。响应带 `nosniff` 与私有缓存头，`Content-Disposition` 为 `inline`。附件字节不参与 `/sync`、历史与搜索：这些接口只返回元数据，客户端按需再取字节。

附件字节保存在服务端 `MEDIA_DIR`（默认相对后端目录的 `../media`），数据库只记录归属与元数据。删除消息与会话不会删除磁盘文件，清理策略尚未实现；当前也不支持上传进度、断点续传、缩略图与视频转码。

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
