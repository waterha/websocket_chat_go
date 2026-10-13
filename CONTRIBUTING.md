# 贡献指南

感谢参与 websocket_chat_go。

## 提交前

1. 从最新主分支创建工作分支。
2. 保持改动聚焦，一个 PR 解决一个问题。
3. 运行 `go test ./...` 和 `npm run build`。
4. 新增 API 时同步更新 README 或接口文档。
5. 不提交 `.env`、数据库转储、密钥或构建产物。

## Pull Request

PR 描述请包含：问题背景、行为变化、测试方式、数据库迁移影响和兼容性说明。涉及安全、权限或消息一致性的改动，请附上失败场景与恢复策略。

## 提交信息

推荐使用 Conventional Commits，例如 `feat: add conversation read cursor`、`fix: reject duplicate message payload`。
