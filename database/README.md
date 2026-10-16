# 数据库说明

`000001_initial` 建立用户、会话、会话成员、登录会话与消息；`000002_im_core` 增加成员已读时间、每设备接收游标、待推送 outbox；`000003_system_messages` 为群成员邀请提示增加消息类型；`000004_friendships` 增加双向好友关系。服务启动时通过 `schema_migrations` 逐项执行尚未应用的迁移，迁移文件内容和版本写入同一事务；既有未版本化初始数据库可识别并升级。生产环境应使用独立迁移任务和最小权限。

- `conversations.next_seq`：会话序号，发送事务锁会话行并加一；`messages` 对 `(conversation_id, seq)` 和 `(sender_id, client_msg_id)` 唯一。
- `messages.kind`：普通消息为 `user`，群成员邀请提示为 `system`；提示与普通消息共用会话序号，可离线同步与历史漫游。
- `conversation_members.last_read_seq/read_at`：用户在一个会话中的单调已读游标。
- `device_cursors.received_seq`：同一用户、会话、设备的单调接收游标，绝不代替已读。
- `message_outbox`：事务内记录成员快照和事件 JSON；未发布事件由单实例 worker 逐批扫描。`published_at` 不是设备确认。
- `friendships`：每个好友关系保存双向两行，添加操作幂等。

开发环境见 `infra/compose.yaml`（PostgreSQL 17、Redis 7）。不要提交真实密码或备份；生产数据库连接必须验证 TLS 证书。删除旧数据和 outbox 保留策略尚未定义。
