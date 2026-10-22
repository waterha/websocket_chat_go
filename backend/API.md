# API 参考

通讯录仅限已登录账号，范围是当前服务器的全部其他账号；当前没有企业租户隔离。`q` 为最多 200 字的大小写不敏感字面子串（`%`、`_` 不作为通配符），`department` 为最多 100 字的精确部门名，留空表示不筛选；`offset` 默认 0，范围 0–1000000；`limit` 默认 50，范围 1–100，非法参数返回 400 `invalid_directory_query`。`departments` 为其他账号已填写的非空部门名，不受当前搜索条件影响；`entries` 中每项包含 `id,email,name,department,job_title`。继续加载时沿用筛选条件并传入 `next_offset`；成员资料实时变化时偏移分页不提供快照保证。工作资料由本人维护，不赋予企业权限，保存会去除首尾空白，拒绝控制字符和超过 100 字的字段（400 `invalid_work_profile`）；不能指定其他用户 ID。

HTTP 前缀 `/api/v1`；成功 `{"data":...}`，失败 `{"error":{"code":"...","message":"..."}}`。受保护接口使用 `session` HttpOnly Cookie。JSON 使用 `snake_case`；同一 Cookie 的会话主体由服务端确定，客户端不能指定发送者。请求体不超过 64 KiB（媒体上传例外）；请求的 `Origin` 必须匹配 `FRONTEND_ORIGIN`（CLI 请求可以省略）。

## 会话、CSRF 与限流

登录与注册会同时下发两个 Cookie：`session`（HttpOnly，不可被脚本读取）与 `csrf`（可被脚本读取）。**除 `GET`/`HEAD`/`OPTIONS` 外的所有请求**都必须带上与 `csrf` Cookie 值一致的 `X-CSRF-Token` 请求头，否则返回 403 `invalid_csrf`。这是防跨站请求伪造的双重提交校验；`SameSite=Lax` 只覆盖大部分表单提交场景，令牌校验才覆盖剩余情况。登录与注册本身无需令牌，因为它们发生在会话建立之前。

- 升级前建立的会话没有 `csrf_token`，因此写请求会返回 401 `reauthentication_required`：请重新登录，而不是被隐式信任。
- 登录失败按账号计数，`IM_TEST_REDIS_ADDR` 中的 Redis 保存固定窗口计数（默认 15 分钟 10 次）。超过后返回 429 `too_many_attempts` 并带 `Retry-After` 头；窗口内的正确密码同样被拒绝。账号不存在与密码错误返回**相同**的 401 `invalid_credentials`，且都计入限流，避免用错误码枚举已注册账号。Redis 不可用时**放行**（fail-open），以免缓存故障把所有人锁在账号外。
- `POST /auth/password`：`{"current_password":"...","new_password":"至少八位"}`。校验当前密码（错误返回 403 `invalid_password`），新旧相同返回 400 `password_unchanged`，新密码长度非法返回 400 `invalid_password`。成功后**撤销除当前会话外的全部会话**，并断开其他设备的 WebSocket 连接；旧密码立即失效。响应 `{"revoked_other_sessions":true}`。
- 会话有效期 7 天，Cookie 为 `SameSite=Lax`；`APP_ENV=production` 时附带 `Secure`。

当前没有：二次验证、找回密码、独立的会话管理界面（列出并逐个撤销设备）、IP 维度限流与审计日志。

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/auth/register` | `{"email":"...","name":"...","password":"至少八位"}`；注册并登录 |
| POST | `/auth/login` | `{"email":"...","password":"..."}` |
| POST | `/auth/logout` | 当前会话注销并断开其 WebSocket |
| POST | `/auth/password` | 修改密码：`{"current_password":"...","new_password":"..."}`；成功后撤销其他会话 |
| GET | `/me` | 当前用户 |
| PATCH | `/me` | 修改当前用户的用户名和用户账号：`{"name":"新用户名","email":"new@example.com"}`；用户 ID 不变，账号重复返回 `email_exists` |
| GET | `/users` | 用户列表（当前最多 1000 条） |
| GET | `/directory?q=工程师&department=研发部&offset=0&limit=50` | 同事通讯录，搜索姓名、账号、部门、职位；不含本人，无需好友关系；返回 `entries,departments,has_more,next_offset` |
| GET | `/directory/me` | 本人的工作资料：`{"department":"","job_title":""}`；未填写时返回空字符串 |
| PATCH | `/directory/me` | 保存本人的工作资料：`{"department":"研发部","job_title":"工程师"}`；两项均为替换值，可传空字符串清空 |
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
| GET | `/conversations/{id}/announcements` | 群公告列表，任意群成员可读，按发布时间倒序 |
| POST | `/conversations/{id}/announcements` | 新建群公告：`{"content":"最多 2000 字"}`；仅群主与管理员 |
| PATCH | `/conversations/{id}/announcements/{announcement_id}` | 编辑群公告：`{"content":"..."}`；仅群主与管理员，作者不变 |
| DELETE | `/conversations/{id}/announcements/{announcement_id}` | 删除群公告；仅群主与管理员 |
| PATCH | `/conversations/{id}/preferences` | 个人会话偏好：`{"pinned":true,"muted":false}`，字段可省略表示不改，仅影响本人 |
| POST | `/conversations/{id}/messages` | `{"client_msg_id":"客户端稳定ID","content":"你好"}`；新消息 201，幂等重复 200。可带 `media_id`（附件）、`reply_to_id`（引用同会话消息）、`mentions`（提及成员 ID 数组）、`blocks`（图文混排，见下文） |
| GET | `/conversations/{id}/messages/{message_id}/thread?limit=100&after_seq=0` | 回复对话窗口：返回根消息与该消息的全部回复（升序分页），见下文 |
| POST | `/conversations/{id}/messages/{seq}/recall` | 撤回消息；发送者本人在 2 分钟内可撤回，群主/管理员可撤回本群任意消息；已撤回的消息内容不再返回 |
| POST | `/conversations/{id}/media?file_name=photo.png` | 上传图片或视频：请求体为文件原始字节，`Content-Length` 必填，`Content-Type` 决定类型；成功 201 返回附件元数据，见下文 |
| GET | `/media/{media_id}` | 下载附件原始字节；仅该会话成员可访问，返回 `Content-Type`、`Content-Disposition: inline`，见下文 |
| GET | `/conversations/{id}/messages?before_seq=100&limit=30` | 从 `before_seq` 之前倒序取页，返回按 `seq` 正序；默认最新 30，最多 100 |
| GET | `/messages/search?q=hello&limit=20&before_time=...&before_id=...` | 跨会话全局搜索：搜索当前用户参与的全部会话，按时间倒序分页，见下文 |
| GET | `/conversations/{id}/messages/search?q=hello&before_seq=100&limit=30` | 仅当前会话成员可搜索，按 `seq` 倒序返回 `messages,has_more,next_before_seq`，详见下文 |
| GET | `/conversations/{id}/sync?after_seq=0&limit=100` | 返回 `messages,last_seq,next_seq,has_more`；按 `seq` 正序取 `after_seq` 之后最多 100 条；下一页用 `next_seq` |
| POST | `/conversations/{id}/read` | `{"seq":42}`；更新成员已读，返回 `conversation_id,user_id,seq,read_at` |
| POST | `/conversations/{id}/received` | `{"device_id":"设备ID","seq":42}`；更新设备接收游标，与已读无关 |
| GET | `/conversations/{id}/messages/{seq}/delivery` | 仅消息发送者可查：返回其他成员中至少一台设备确认到该序号的人数、其他成员总数及查询时间；找不到或无权查看返回 404 |
| GET | `/devices/{device_id}/cursors` | 当前设备对每个会话的 `conversation_id,device_id,received_seq,last_seq`；用最新序号对账 |
| POST | `/presence` | `{"user_ids":["用户ID"]}`（最多 1000 个）返回 `{"online":{"用户ID":true}}`；仅表示该用户当前是否有活动连接 |

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
- **撤回**：`POST /conversations/{id}/messages/{seq}/recall`。发送者本人可在**发送后 2 分钟内**撤回，超时返回 403 `recall_window_expired`；群主与管理员不受该窗口限制，可撤回本群任意成员的消息（管理场景）。其他人返回 403 `forbidden`。撤回**保留消息行与会话序号**，因此历史仍然连续、`/sync` 游标不受影响；`content`、附件元数据与 **`media_id` 都不再返回**，客户端应显示占位文本。重复撤回幂等。**撤回会一并使附件不可下载**：`GET /media/{id}` 对该附件返回 404，即使调用者仍是会话成员且保存了该 id。附件字节由保留清理延迟回收（见下），磁盘删除不影响消息行与撤回状态。
- 幂等比较包含 `content`、`media_id`、`reply_to_id` 与 `mentions`，其中任一不同而 `client_msg_id` 相同会返回 409 `message_conflict`。
- 撤回推送 `message.recalled` 事件（载荷为撤回后的消息），与 `message.created` 一样可能重复或丢失，客户端应通过 `/sync` 兜底。

当前没有：撤回后还原、管理员撤回审计记录、提及的定向通知与免打扰规则、@所有人。



## 会话偏好

`PATCH /conversations/{id}/preferences` 设置**调用者本人**的置顶与免打扰。两个字段都是可选的布尔值：省略表示保持不变，显式传 `false` 表示清除，因此客户端可以只提交用户改动的那一项。返回 `{"conversation_id":"...","pinned":bool,"muted":bool}`。

- 偏好保存在成员关系行上（`conversation_members.pinned_at/muted`），**其他成员完全看不到**，也不会改变他人的会话列表或提醒；非成员或会话不存在均返回 404 `not_found`。
- `GET /conversations` 返回每个会话的 `pinned` 与 `muted`，并把置顶会话排在前面（`ORDER BY (pinned_at IS NULL), pinned_at DESC`），使多个置顶会话按置顶时间稳定排序。
- 免打扰只影响客户端的提示与未读角标，**不影响消息投递、接收游标与已读状态**。

偏好是本机可见的视图设置，因此不产生 outbox 事件，也不做广播。**归档功能已移除**（迁移 000014）：它只增加第二个生命周期，却不改变任何成员能看到或能做的事，因此不值得保留。当前也没有：按会话自定义提醒级别、免打扰时间段与文件夹分组。

## 图文混排消息（blocks）

一条消息可以按发送者书写的顺序包含多张图片与多段文字，因此「先说明、再贴图、再补充、再贴图」能保持原样显示。

- 请求体用 `blocks` 数组表示顺序，元素为 `{"type":"text","text":"..."}` 或 `{"type":"image","media_id":"..."}`。`content` 由服务端从所有 `text` 段用换行拼接得到，因此客户端不必同时传 `content`；传了也会被覆盖。使用 `blocks` 时 `media_id` 会被清空，避免同一条消息被渲染两次。
- 服务端校验：最多 20 段；`text` 段去除首尾空白后为空的会被丢弃；文字合计不超过 4000 字符；`type` 只接受 `text` 与 `image`；`image` 段必须带 `media_id`，且该附件必须属于发送者本人并在同一会话，否则返回 404 `not_found`。
- `image` 段的 `file_name`、`media_type`、`content_type`、`byte_size` **由服务端从数据库填充**，客户端传入的值会被忽略，因此不能伪造附件名或类型。
- 读取时每条消息带 `blocks`（顺序与写入一致）；`blocks` 为空的响应不带该字段，表示按旧的单附件或纯文本方式渲染。撤回会同时清空 `blocks`，因为每个 `image` 段都指向一个被撤回的附件。
- 幂等重试会比较 `blocks` 的段类型、文字与 `media_id`，任一段不同即返回 409 `message_conflict`。

## 回复对话窗口（话题串）

回复被组织成**扁平的话题串**：一条被回复的消息是窗口的根，对它（以及对其回复）的所有回复都属于同一个窗口。这样「展开一条消息的全部回复」是一次带索引的查询，而不会随嵌套深度增长。

- 根由服务端推导，不信任客户端：发送回复时 `thread_root_id` 由被引用消息决定。若被引用消息自己有根，则沿用该根（回复的回复归入同一窗口）；否则以被引用消息为根（开启新窗口）。因此**客户端传入 `thread_root_id` 不会生效**，无法把回复塞进无关窗口。
- `GET /conversations/{id}/messages/{message_id}/thread` 返回 `{"root":{...},"replies":[...],"has_more":bool,"next_after_seq":N,"reply_count":N}`。`message_id` 可以是根，也可以是窗口内的任一回复——服务端会解析到根，因此从回复打开得到的是同一个窗口，而不是嵌套窗口。`replies` 按会话内 `seq` **升序**返回（旧的在前）。`limit` 默认 100、上限 100；`after_seq` 为上一页的 `next_after_seq`。非成员、非本会话消息与不存在的消息都返回 404 `not_found`。
- 主聊天界面的每条根消息带 `thread` 摘要：`{"root_id","reply_count","preview":[...]}`。`reply_count` 是**窗口内回复总数**，而 `preview` 只保留**最早 3 条**回复，避免长话题把历史列表撑大。没有回复的消息不带 `thread` 字段。摘要在历史与 `/sync` 返回消息时一并附带，因此客户端不需要为每条消息单独请求。
- 窗口内的回复也可以被回复和撤回，规则与其他消息相同；窗口本身不改变撤回语义。
- **客户端行为**：点击消息上的「回复」直接打开该消息的对话窗口，回复在主聊天输入框之外、由窗口自己的输入框提交，两者互不干扰。窗口内提交的回复以窗口根消息为 `reply_to_id`，因此始终落在同一窗口。主输入框发送的消息不带引用（引用统一由窗口产生）。

**发送路径的一致性**：`message.send` WebSocket 命令的 `data` 就是客户端待确认发送记录本身，该记录必须包含服务端读取的全部字段（`client_msg_id`、`content`、`media_id`、`reply_to_id`、`mentions`）。曾出现过记录只存 `content`/`media_id` 导致 **WebSocket 发送静默丢失引用与提及**、而 HTTP 回退仍正常的问题；现由测试 `TestReplySurvivesTheWebsocketSendPath` 固定该行为。

当前没有：话题串成员订阅、未读计数、把话题串限制在频道内、以及回复的通知聚合。

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

移除成员、退群、角色变更与群主转让都会写入一条 `kind='system'` 的系统提示并复用会话序号，因此和其他消息一样可以被离线成员补拉漫游。**移除成员不删除其历史消息**：`messages` 不再要求发送者当前仍是成员（发送时由应用层校验成员资格），所以群内历史仍然完整、可搜索。群公告是独立记录的集合（`announcements` 表）：群主与管理员可新建、编辑、删除每一条，普通成员只读全部列表；正文 1–2000 字符、每群最多 100 条，`PATCH` 保留原作者并把 `edited` 置真，跨会话或未知 id 返回 404。角色与群主写在 `conversation_members.role` 与 `conversations.created_by`。

`GET /conversations` 的成员对象带 `role` 字段并按「群主、管理员、其他成员」排序，会话对象带 `announcement` 与 `owner_id`。群元数据变化时向全群推送 `conversation.updated`，客户端据此刷新成员列表与公告。当前没有：自定义权限、禁言、成员邀请审批、群分组与审计日志。管理员不能移除其他管理员是刻意设计，避免管理员互相驱逐；如需该能力应由群主调整角色后操作。



## 附件：图片、视频与文件

消息支持 `kind` 为 `image`、`video` 或 `file` 的附件消息。附件先上传再发送：`POST /conversations/{id}/media` 接受图片、视频或任意类型的文件，`GET /media/{media_id}` 只在调用者是所属会话成员时返回字节。**上传者与发送者必须是同一人**，且附件必须属于目标会话，因此上传凭证不能被借用。

- 类型由 `Content-Type` 决定并**原样存储与回传**：`image/jpeg`、`image/png`、`image/gif`、`image/webp` 与 `video/mp4`、`video/webm`、`video/ogg` 作为内联媒体；其他**语法合法**的 `type/subtype`（例如 `application/pdf`、`application/zip`、`text/plain`）作为通用文件。空值、超过 128 字符、含空白或控制字符、缺少 `type/subtype` 结构的值返回 415 `unsupported_media_type`。服务端**不嗅探文件内容**，声明的类型即存储与下载类型；下载方必须把返回内容视为不可信数据。
- 大小上限：图片 10 MiB、视频 100 MiB、其他文件 50 MiB；超限返回 413 `media_too_large`。缺失或非正的 `Content-Length` 返回 400 `invalid_upload`。
- 上传非成员会话返回 404 `not_found`。成功 201 返回 `{"id","conversation_id","file_name","media_type","content_type","byte_size","created_at"}`；`file_name` 已去除路径部分并截断到 255 个字符。存储键不出现在任何响应中。
- 发送附件消息：`{"client_msg_id":"...","content":"可选说明","media_id":"上传返回的 id"}`。`kind` 由附件类型推导，不读取客户端声明；`content` 可为空字符串（最多 4000 字符）作为说明。引用不存在、不属于该会话或非本人上传的 `media_id` 返回 404；`content` 为空且没有 `media_id` 返回 400。
- `GET /media/{media_id}`：`image`/`video` 以 `Content-Disposition: inline` 返回，`file` 以 `attachment` 返回并带 UTF-8 文件名，避免浏览器在应用源内渲染不可信文档。非成员或不存在返回 404 `not_found`。响应带 `nosniff` 与私有缓存头。

附件字节保存在服务端 `MEDIA_DIR`（默认相对后端目录的 `../media`），数据库只记录归属与元数据。**保留清理**每 5 分钟回收两类数据：已发布且超过保留期（默认 7 天）的 outbox 行，以及已被撤回消息所引用的附件字节（行保留并标记 `bytes_removed_at`，文件删除）。未发布的 outbox 行与仍被正常消息引用的附件**永不删除**。清理是幂等的，删除文件后写标记，进程在两步之间崩溃时下一轮会重试。当前不支持上传进度、断点续传、缩略图、视频转码与病毒扫描。

## 在线状态

`POST /api/v1/presence` 用 `{"user_ids":[...]}` 查询这些用户当前是否有活动连接，返回 `{"online":{"用户ID":true}}`；未列出或已过期的用户视为离线。它读取的是 WebSocket 写循环每 `Heartbeat` 刷新的 Redis 标记（TTL 为三倍心跳），因此回答的是「最近一个心跳窗口内是否有连接」，而不是投递保证，也不是历史在线记录。`GET /conversations` 的成员对象也带 `online` 字段，方便直接渲染。

查询是**只读**的，不产生事件；Redis 不可用时该字段与接口会失败或缺失，客户端不应据此断言离线。当前没有：在线状态的推送通知、自定义状态（忙碌/离开）、最后上线时间与离线原因。

## 全局搜索

`GET /api/v1/messages/search?q=预算&limit=20` 搜索**当前用户参与的全部会话**，包含系统提示，排除已撤回的消息。非成员的消息永远不会出现，因此结果不会泄露他人会话。

- `q` 必填，去除首尾空白后为 1–100 个 Unicode 字符；大小写不敏感，`%`、`_` 与反斜杠均按字面处理。非法返回 400 `invalid_query`。
- `limit` 可省略，默认 20，必须为 1–50，否则 400 `invalid_limit`。
- 分页游标是 `(created_at, id)` 对：`before_time` 为 RFC 3339 时间，`before_id` 为消息 ID，**两者必须同时提供**。之所以不用 `seq`，是因为 `seq` 只在单个会话内连续，无法跨会话排序。缺少其一或时间格式错误返回 400 `invalid_cursor`。首页不传游标（服务端以 SQL NULL 表示无游标）。
- 成功返回 `{"results":[{"message":{...},"conversation_name":"...","conversation_kind":"group|direct","member_count":3}],"has_more":bool,"next_before_time":"...","next_before_id":"..."}`。结果是完整消息对象（含附件与引用元数据），按 `created_at DESC, id DESC` 排列，下一页使用返回的游标。空页固定返回 `has_more=false`，不应再翻页。

会话内搜索仍是独立接口 `/conversations/{id}/messages/search`，按会话内 `seq` 分页。当前全局搜索没有：按成员或时间范围过滤、独立的全文索引排序（使用 `pg_trgm` 的 GIN 索引加速字面匹配）与结果高亮由客户端完成。

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
