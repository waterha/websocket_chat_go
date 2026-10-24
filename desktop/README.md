# Windows 桌面端

桌面程序使用 **Go + Wails + Windows WebView2**。它复用 `web/` 中的完整聊天应用，以系统 WebView 显示界面，不将 Chromium 打包进程序。

## 一键本机运行

双击 `desktop/build/bin/WebsocketChat.exe` 即可，无需先运行任何脚本。当“服务器设置”中的地址是本机 HTTP 地址（默认 `http://127.0.0.1:8080`）且该端口上还没有聊天服务时，启动器会在窗口后台自动完成整个本机服务栈：

- 启动私有 PostgreSQL（端口 5434；数据目录在应用数据目录的 `pgdata` 下，首次运行自动 `initdb` 并创建应用账号与数据库）
- 启动 Redis（端口 6380；未安装时自动下载便携版到应用数据目录的 `redis` 下）
- 在进程内运行聊天服务（监听端口与服务器地址一致），并应用 `database/migrations` 的数据库结构

窗口顶部显示每个步骤的进度；失败时给出原因和“重试”按钮。由启动器拉起的服务在关闭窗口时一并停止。若端口上已有聊天服务在运行则直接复用；启动器自己的数据库和 Redis 使用 5434/6380，与开发脚本的 5433/6379 互不影响。

启动器要求本机装有 PostgreSQL 12+：按 `IM_POSTGRES_BIN`、`PATH`、Program Files 和各磁盘根目录的顺序查找 `pg_ctl`。Redis 不需要预装。数据库与 Redis 端口可用环境变量 `IM_DESKTOP_PGPORT`、`IM_DESKTOP_REDIS_PORT` 覆盖。

应用数据（pgdata、redis-data、media、migrations、logs）优先存于 `%APPDATA%\WebsocketChat`；不可写时自动使用程序旁的 `WebsocketChat-data`，再尝试系统临时目录中的同名目录。

## 构建

使用 PowerShell 构建 Windows x64 可执行文件和分发压缩包：

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\build-desktop.ps1
```

构建产物：

- `desktop/build/bin/WebsocketChat.exe`
- `desktop/build/bin/WebsocketChat-windows-amd64.zip`

构建前脚本会把 `database/migrations` 复制到 `desktop/migrations`（git 忽略，仅提交 `.keep` 占位），聊天服务的数据库结构随启动器一起发布。

构建机需要 Go 1.26、Node.js 22 以上及网络访问，以安装 Wails CLI 和前端依赖。用户机器不需要安装 Go、Node.js 或 Redis，但需要 Windows 10/11、WebView2 Runtime，以及本机 PostgreSQL 12+（一键本机运行时）或可访问的聊天服务器。

## 连接团队服务器

首次打开应用时，在窗口顶部的“服务器设置”中输入聊天服务器的基础 HTTP 或 HTTPS 地址，测试连接并保存。远程或 HTTPS 地址不会触发本机服务启动；保存本机 HTTP 地址后会自动启动上面的服务栈。聊天服务要求请求来源与已配置的 `FRONTEND_ORIGIN` 一致；本机服务默认使用高级设置 `http://127.0.0.1:4173`。

设置、WebView 缓存和聊天草稿优先存于 `%APPDATA%\WebsocketChat`。登录 Cookie 仅留在运行中进程的内存里；关闭应用后重新登录即可。待确认消息、接收设备 ID 和已读位置按服务器地址隔离保存在本机。图片在系统 WebView 中按需加载，点击图片与文件时通过系统“另存为”保存附件。单实例锁可避免重复窗口和设备连接。

登录后点击侧边栏“同事通讯录”，可以按姓名、账号、部门和职位查找本服务器的同事、按部门筛选并直接发消息。“我的工作资料”用于维护自己的部门和职位，这些信息由本人填写。连接远程服务器时，服务端需同步升级到包含迁移 `000019` 的版本；一键本机启动会在服务启动时自动执行迁移。尚不提供部门层级、企业管理员或多企业隔离。

桌面网络适配、本机服务管理与配置保存测试：

```powershell
Set-Location desktop
go test ./...
go vet ./...
```
