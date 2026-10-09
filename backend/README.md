# Go 后端：由你从零编写

所有 Go 源码（包括仅有包声明的占位文件）及 `go.mod` 已删除。

本目录仅保留本说明及 `API.md` 接口契约建议，不包含可运行的后端。你可以自行初始化 Go 模块，选择目录结构、框架和 PostgreSQL 驱动，不需要遵循之前的占位结构。

保留的 `../infra/compose.yaml` 与 `../.env.example` 仅用于 PostgreSQL 开发环境；`../database/migrations/` 下的 SQL 文件为空，业务表由你设计。

前端独立运行；注册和登录目前只在浏览器本地演示，尚未请求接口或建立 WebSocket 连接。你完成后端后，再替换 `../frontend/app.js` 中的本地演示数据与发送逻辑。`API.md` 是参考文档，不代表接口已实现。
