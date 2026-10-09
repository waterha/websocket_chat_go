# PostgreSQL 预留目录

本次仅切换数据库目标及开发环境配置，不实现后端，不建立业务表，也不迁移原 MySQL 数据。

- `migrations/000001_initial.up.sql`：空文件，由你填写建表 SQL。
- `migrations/000001_initial.down.sql`：空文件，由你填写对应回滚 SQL。
- `../infra/compose.yaml`：仅启动 PostgreSQL，不启动 Go 服务或 Redis。
- `../.env.example`：连接参数模板；`PGSSLMODE=disable` 仅用于本机开发。

建议业务模型为用户、好友关系、会话、会话成员、消息。消息按会话查询，使用游标分页。表结构、约束、权限和事务由你确定。

SQL 占位文件不会自动挂载或执行。你写完以后，再选择迁移工具或使用 `psql` 手动执行；不要把空文件误当作已完成的数据库迁移。
