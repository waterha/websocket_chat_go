# websocket_chat_go

完全重构的暖色即时通讯前端。旧 Go / MySQL / Redis 实现、静态页面、配置和生成文件已移除；保留 Git 历史及原许可证。新的数据库目标为 PostgreSQL，后端源码已全部删除，由你独立编写。

## 当前交付边界

- 前端：原生 HTML、CSS、JavaScript，零构建依赖、无第三方远程资源。
- 配色：奶油白、陶土红、杏色，辅以低饱和鼠尾草绿。
- 交互：邮箱密码注册/登录演示、退出账号、会话切换、联系人/群组列表、未读/群聊筛选、消息搜索、文本/表情发送、附件预览、本地群组创建、资料编辑、置顶、静音偏好、清空、导出与重置。
- 界面：左侧导航和联系人，右侧聊天记录与输入框；没有全局顶部栏、右侧详情栏、公告、装饰卡片或页脚。手机采用联系人/聊天切换。
- 数据：全部为演示数据；账号摘要、登录会话、本地消息、会话、资料和偏好使用浏览器 localStorage。真实环境必须改为服务端会话，不能把浏览器本地认证当作生产安全方案。不同浏览器或不同访问地址的数据不共享。
- 后端：所有 Go 源码、模块文件和可运行服务文件均已删除；`backend/` 仅保留接口参考文档，不包含可运行代码。
- 数据库：提供 PostgreSQL 开发配置和空 SQL 迁移文件。未创建业务表、未迁移旧数据。

## 立即预览

在项目根目录执行：

```powershell
python -m http.server 4173 --bind 127.0.0.1 --directory frontend
```

浏览器访问 `http://127.0.0.1:4173`。也可直接双击 `frontend/index.html`，但推荐使用本地 HTTP 地址，以获得稳定的存储和浏览器能力支持。

快捷键：Ctrl / ⌘ + K 搜索会话；Enter 发送、Shift + Enter 换行。设置里关闭 Enter 发送后，使用 Ctrl / ⌘ + Enter。中文输入法确认选词不会直接发送消息。

图片支持 PNG、JPEG、WebP、GIF，最多 2 MB，实际图片保存到浏览器本地。其他附件最多 5 MB，仅保存文件名和大小，不上传、不保留文件内容；页面不会提供虚假的下载入口。浏览器空间已满会提示，请导出备份或移除大图片。

演示里的在线状态、预置消息是静态样例，不代表真实用户活动。会话菜单中的静音操作只保存偏好，没有真实推送或通知。没有自动回复、真正的登录和多端同步。

## 文件结构

```text
frontend/
  index.html                    页面布局
  styles.css                    暖色主题与响应式布局
  app.js                        前端本地演示交互
  assets/favicon.svg            标识
  assets/sunset.svg              本地日落插画
backend/
  README.md                     后端重写说明
  API.md                        后续对接契约建议
database/
  README.md
  migrations/000001_initial.up.sql    空建表迁移
  migrations/000001_initial.down.sql  空回滚迁移
infra/compose.yaml              仅 PostgreSQL 开发环境
.env.example                    环境模板
```

## PostgreSQL 开发环境

先复制环境模板，替换示例密码；不要把真实 `.env` 提交到仓库。

```powershell
Copy-Item .env.example .env
docker compose --env-file .env -f infra/compose.yaml up -d
docker compose --env-file .env -f infra/compose.yaml ps
```

开发实例只绑定本机 `127.0.0.1:5433`，避免与本地默认端口冲突。数据库和用户默认为 `websocket_chat_go`。已有的 PostgreSQL 数据卷不会因修改环境变量自动重建账号或密码。

如果不用 Docker，可以自行在本机 PostgreSQL 创建开发账号与数据库，并相应调整环境模板。当前前端不需要数据库，代码也不会替你读取 `.env` 或自动连接。

迁移 SQL 当前为空，由你填写之后再手动执行或接入迁移工具。`PGSSLMODE=disable` 仅用于本机；生产环境应验证证书、限制权限并独立管理凭据。

## 你编写后端时

可按 `backend/README.md` 与 `backend/API.md` 自行初始化 Go 服务，再替换 `frontend/app.js` 中的 `people`、`conversations`、`baseMessages`、`persist()`、`sendMessage()` 等本地数据入口。当前没有隐藏的远程接口调用，也没有需要沿用的旧项目框架。

真实消息发送成功必须以服务端 ACK 为准；不能把当前“仅本地”状态当作已送达、已读。群组权限、接收者校验、上传校验、幂等和事务必须在 Go 后端实现。
