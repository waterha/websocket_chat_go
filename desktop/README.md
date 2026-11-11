# Windows 桌面端

桌面程序使用 **Go + Wails + Windows WebView2**。它复用 `web/` 中的完整聊天应用，以系统 WebView 显示界面，不将 Chromium 打包进程序。

## 一键本机运行

双击 `desktop/build/bin/WebsocketChat.exe` 即可，无需先运行任何脚本。当“服务器设置”中的地址是本机 HTTP 地址（默认 `http://127.0.0.1:8080`）且该端口上还没有聊天服务时，启动器会在窗口后台自动完成整个本机服务栈：

- 从 EXE 内释放便携 PostgreSQL 16 和必要的微软 VC DLL，启动私有数据库（端口 5434；数据目录在应用数据目录的 `pgdata` 下，首次运行自动 `initdb` 并创建应用账号与数据库）
- 从 EXE 内释放并启动 Redis（端口 6380），首次运行同样不需要联网下载
- 在进程内运行聊天服务（监听端口与服务器地址一致），并应用 `database/migrations` 的数据库结构

窗口顶部显示每个步骤的进度；失败时给出原因和“重试”按钮。由启动器拉起的服务在关闭窗口时一并停止。若端口上已有聊天服务在运行则直接复用；启动器自己的数据库和 Redis 使用 5434/6380，与开发脚本的 5433/6379 互不影响。

**用户不需要预装 PostgreSQL、Redis、VC 运行库、Go、Node.js，也不需要运行任何项目脚本。** 默认先使用 EXE 内置的私有依赖，避免扫描系统盘或依赖开发机的 PATH。首次释放归档会稍慢，后续启动复用版本/校验值对应的缓存。数据库与 Redis 端口可用高级环境变量 `IM_DESKTOP_PGPORT`、`IM_DESKTOP_REDIS_PORT` 覆盖；显式设置 `IM_POSTGRES_BIN`、`IM_REDIS_DIR` 时仍由使用者指定的运行时负责。

已有 PostgreSQL 16 数据可以直接复用内置的同主版本更新；若数据属于其他主版本，程序不会强制升级或删除数据，而是要求使用匹配的原运行时。运行时安装采用 SHA256 校验、临时目录和完整后原子改名，不会把半下载/半解压文件标记为可用；重试可恢复缺失的运行时文件，用户数据库、附件与设置不受影响。

应用数据（pgdata、redis-data、media、migrations、logs）优先存于 `%APPDATA%\WebsocketChat`；不可写时自动使用程序旁的 `WebsocketChat-data`，再尝试系统临时目录中的同名目录。

## 构建

使用 PowerShell 构建 Windows x64 可执行文件和分发压缩包：

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\build-desktop.ps1
```

构建产物：

- `desktop/build/bin/WebsocketChat.exe`
- `desktop/build/bin/WebsocketChat-windows-amd64.zip`

构建前脚本会把 `database/migrations` 复制到 `desktop/migrations`（git 忽略，仅提交 `.keep` 占位），聊天服务的数据库结构随启动器一起发布。Wails 随后根据 `desktop/wails.json` 执行 `web/package.json` 中的 `build:desktop`，重新生成 `desktop/frontend/dist`，最后将迁移和前端资源一起嵌入可执行文件。直接 `go build` / `go test` 不会同步这些资源；修改共享前端或新增迁移后，发布必须走完整构建流程，不能手工编辑生成目录。全局搜索索引需要包含 `000031_global_search_indexes`；旧的本地生成目录或已分发的 EXE 不会因源码更新而自动升级。

构建机需要 Go 1.26、Node.js 22 以上及网络访问。正式构建先执行 `scripts/prepare-desktop-runtimes.ps1`，下载并校验固定版本的 EDB PostgreSQL、redis-windows 和微软 VC 官方包，只保留运行所需内容与许可，生成被 git 忽略的嵌入归档和校验清单。上游固定 URL 版本发生变化或哈希不符时停止构建，不能静默替换依赖。

用户机器只需要 Windows 10/11 x64 和 Microsoft Edge WebView2。通常系统已有 WebView2；缺少时 Wails 启动器会提示下载安装（这一特殊步骤需要联网）。数据库与 Redis 不需用户下载安装，默认本机模式不需要先启动其他服务。单 EXE 包体会增大，这是携带离线依赖的预期成本。

## 连接团队服务器

首次打开应用时，在窗口顶部的“服务器设置”中输入聊天服务器的基础 HTTP 或 HTTPS 地址，测试连接并保存。远程或 HTTPS 地址不会触发本机服务启动；保存本机 HTTP 地址后会自动启动上面的服务栈。聊天服务要求请求来源与已配置的 `FRONTEND_ORIGIN` 一致；本机服务默认使用高级设置 `http://127.0.0.1:4173`。

设置、WebView 缓存和聊天草稿优先存于 `%APPDATA%\WebsocketChat`。登录 Cookie 仅留在运行中进程的内存里；关闭应用后重新登录即可。待确认消息、接收设备 ID 和已读位置按服务器地址隔离保存在本机。图片在系统 WebView 中按需加载，点击图片与文件时通过系统“另存为”保存附件。单实例锁可避免重复窗口和设备连接。

登录后点击侧边栏“同事通讯录”，可以按姓名、账号、部门和职位查找本服务器的同事、按部门筛选并直接发消息。“我的工作资料”用于维护自己的部门和职位，这些信息由本人填写。连接远程服务器时，服务端需同步升级到包含迁移 `000019` 的版本；一键本机启动会在服务启动时自动执行迁移。尚不提供部门层级、企业管理员或多企业隔离。

## 原生侧资源与性能回归

- 启动健康轮询读完小型响应后复用 HTTP 连接，最多额外读取 64 KiB；2 秒请求超时保留，不会无限排空未知服务的响应。`TestHealthyReusesConnectionAcrossPolls` 使用真实 TCP 连接计数，验证带 JSON 响应的连续 10 次轮询仅建立 1 条连接。
- WebSocket 在拨号前预留连接 ID，重复调用不再发起多余握手；关闭尚在连接中的 socket 会立即取消拨号，而不是等 10 秒超时。`TestPendingSocketDeduplicatesAndCancelsHandshake` 验证重复请求只有 1 次服务端握手、关闭后 2 秒测试期限内返回，以及取消后可重用 ID。
- 关闭客户端时先在锁内获取连接快照，再于锁外关闭，避免连接释放与读循环清理争用注册表锁。原生 Cookie、CSRF、Origin、重定向隔离和附件流式传输保持不变；不更改 WebView 安全选项。

桌面网络适配、本机服务管理与配置保存测试：

```powershell
Set-Location desktop
go test ./...
go vet ./...
```

离线依赖首次使用的真实集成验证（先运行完整构建或运行时准备脚本）：

```powershell
$env:IM_TEST_BUNDLED_RUNTIME = '1'
go test ./internal/local -run '^TestStandaloneBundledRuntimeFirstLaunchAndRestart$' -count=1 -v -timeout=8m
```

该测试不复用开发服务：使用新建的中文含空格数据目录、动态空闲端口及内置运行时，检查初始化、注册登录、会话接口、关闭后的端口回收、再次启动登录同一账户。Windows 数据库工具的 ANSI 路径问题通过短路径或受当前用户私有 ACL 保护的 ASCII 目录别名自动处理，不移动或删除原数据。
