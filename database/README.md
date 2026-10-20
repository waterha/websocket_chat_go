# 数据库说明

`000001_initial` 建立用户、会话、会话成员、登录会话与消息；`000002_im_core` 增加成员已读时间、每设备接收游标、待推送 outbox；`000003_system_messages` 为群成员邀请提示增加消息类型；`000004_friendships` 增加双向好友关系；`000005_friend_requests` 增加带可选申请信息的好友申请及 pending/accepted/rejected 状态；`000006_message_history_index` 移除重复历史索引，保留 `(conversation_id, seq)` 唯一 B-tree 同时服务正向补拉和反向历史分页；`000007_media_messages` 增加图片/视频附件表并把消息类型扩展为 `user/system/image/video`；`000008_group_roles` 增加群主/管理员角色、群公告与 `created_by`，并移除「消息发送者必须是当前成员」的外键；`000009_message_reply_mentions` 为消息增加引用回复、@提及与撤回状态；`000010_file_attachments_search` 允许通用文件附件并增加 `pg_trgm` 内容索引。服务启动时通过 `schema_migrations` 逐项执行尚未应用的迁移，迁移文件内容和版本写入同一事务；既有未版本化初始数据库可识别并升级。生产环境应使用独立迁移任务和最小权限。

- `conversations.next_seq`：会话序号，发送事务锁会话行并加一；`messages` 对 `(conversation_id, seq)` 和 `(sender_id, client_msg_id)` 唯一。
- `messages.kind`：普通消息为 `user`，群成员邀请提示为 `system`，附件消息为 `image` 或 `video`；提示与普通消息共用会话序号，可离线同步与历史漫游。
- `message_media`：一次上传的归属（上传者、会话）、文件名、媒体类型（`image`/`video`/`file`）、MIME、字节数与磁盘存储键。`messages.media_id` 外键指向它，并对非空值建唯一索引，使一份上传最多只能被一条消息引用。磁盘文件不随消息或会话删除，清理策略尚未实现。
- `messages_content_trgm_idx`：`pg_trgm` 的 GIN 索引，加速全局搜索的字面 `strpos` 匹配。迁移会创建 `pg_trgm` 扩展，因此执行迁移的账号需要相应权限；无该扩展的托管环境需先在实例上启用。
- `conversations.created_by` / `announcement`：群主（转让时同步更新）与群公告（最多 2000 字）。
- `conversation_members.role`：`owner`/`admin`/`member`；部分唯一索引 `conversation_members_owner_idx` 保证每个会话最多一个群主。私聊成员固定为 `member`。
- 消息发送者不必是当前成员：`messages` 不再有 `(conversation_id, sender_id)` 外键，因此移除成员或退群不会删除、也无法删除群内历史；写入新消息时的成员资格由应用层在事务内校验。
- `messages.reply_to_id` / `mentions` / `recalled_at`：引用回复的同一会话消息、被提及的成员 ID 数组（GIN 索引）、撤回时间。撤回只置时间戳并保留行与序号，使历史连续；内容与附件在读取时被隐藏。
- `conversation_members.last_read_seq/read_at`：用户在一个会话中的单调已读游标。
- `device_cursors.received_seq`：同一用户、会话、设备的单调接收游标，绝不代替已读。
- `message_outbox`：事务内记录成员快照和事件 JSON；未发布事件由单实例 worker 逐批扫描。`published_at` 不是设备确认。
- `friendships`：每个好友关系保存双向两行，同意好友申请时写入。
- `friend_requests`：保存好友申请、可选申请信息和处理状态；同一方向同时只能有一条待处理申请。

开发环境见 `infra/compose.yaml`（PostgreSQL 17、Redis 7）。不要提交真实密码或备份；生产数据库连接必须验证 TLS 证书。删除旧数据和 outbox 保留策略尚未定义。

## 历史存储与索引

消息继续以 PostgreSQL 为权威存储；按会话只保存一份消息正文，不为每个成员复制。`UNIQUE (conversation_id, seq)` 创建的 B-tree 在固定会话时可以正向及反向扫描：`/sync` 用 `after_seq` 升序读取，历史接口用排他的 `before_seq` 倒序取页后以升序返回。两者每页最多 100 条，均无需深 `OFFSET`，无需额外维护同列的降序历史索引。选型比较、一手来源和扩容边界见 [存储调研](../docs/research/2026-10-18-message-history-storage.md)。

`000006` 不改消息数据或唯一性约束。其 down 脚本重建旧索引；当前迁移执行器只自动应用 up 脚本，不提供自动降级命令。普通 `DROP INDEX` 会取得表级强锁，回滚的 `CREATE INDEX` 也会阻塞写入；繁忙数据库应在独立迁移维护窗口执行。若另行设计在线操作，`DROP INDEX CONCURRENTLY` 必须在事务块外运行，不能直接用于当前事务式迁移执行器。

历史存储专项测试只依赖独立 PostgreSQL 数据库，测试自动创建并清理各自 schema：

```powershell
Set-Location backend
$env:IM_TEST_DATABASE_URL='postgres://测试用户:测试密码@127.0.0.1:5433/测试库?sslmode=disable'
go test -v ./internal/logic -run TestHistoryStorage
```

测试覆盖跨页期间追加消息、连续补拉、会话隔离、空页边界、索引回滚/升级和带实际执行数据的正反向查询计划。查询计划断言针对测试数据分布，不能视为生产查询性能承诺。完整 API 集成测试仍需同时设置 `IM_TEST_REDIS_ADDR`。
