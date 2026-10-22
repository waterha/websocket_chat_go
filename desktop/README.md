# Windows 桌面端

桌面程序使用 **Go + Wails + Windows WebView2**。它复用 `web/` 中的完整聊天应用，以系统 WebView 显示界面，不将 Chromium 打包进程序。数据库、Redis 和业务 API 仍由服务端托管；桌面程序保存服务器地址，登录后连接现有服务。

双击项目根目录的 `启动桌面端.bat` 可在本机启动服务并打开桌面窗口。此操作会用现有本机开发服务：PostgreSQL、Redis、Go、Node.js 和 Microsoft Edge WebView2 Runtime。

需要手动打开已有服务器时，双击 `desktop/build/bin/WebsocketChat.exe` 即可。该程序独立运行，不会替你启动本地数据库。

双击 `构建桌面端.bat` 可打包 Windows x64 可执行文件和分发压缩包：

- `desktop/build/bin/WebsocketChat.exe`
- `desktop/build/bin/WebsocketChat-windows-amd64.zip`

构建机需要 Go 1.26、Node.js 22 以上及网络访问，以安装 Wails CLI 和前端依赖。用户机器不需要安装 Go、Node.js 或数据库，但需要 Windows 10/11、WebView2 Runtime，以及可访问的聊天服务器。首次构建会把构建工具缓存在 git 忽略的 `.dev/tools/` 中。

首次打开应用时，在窗口顶部的“服务器设置”中输入聊天服务器的基础 HTTP 或 HTTPS 地址，测试连接并保存。支持线上服务器或 `http://127.0.0.1:8080` 本机服务器。聊天服务要求请求来源与已配置的 `FRONTEND_ORIGIN` 一致；使用本机服务时通常填写高级设置 `http://127.0.0.1:4173`。

设置、WebView 缓存和聊天草稿存于 `%APPDATA%\WebsocketChat`。登录 Cookie 仅留在运行中进程的内存里；关闭应用后重新登录即可。待确认消息、接收设备 ID 和已读位置按服务器地址隔离保存在本机。图片在系统 WebView 中按需加载，点击图片与文件时通过系统“另存为”保存附件。单实例锁可避免重复窗口和设备连接。

桌面网络适配与配置保存测试：

```powershell
Set-Location desktop
go test ./...
go vet ./...
```
