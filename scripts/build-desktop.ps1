param([switch]$SkipInstall)

$ErrorActionPreference = 'Stop'
$ProjectRoot = Split-Path -Parent $PSScriptRoot
$DesktopRoot = Join-Path $ProjectRoot 'desktop'
$ToolsDir = Join-Path $ProjectRoot '.dev\tools\wails-v2.11.0'
$GoTmp = Join-Path $ProjectRoot '.dev\go-tmp'
$GoCache = Join-Path $ProjectRoot '.dev\go-cache'
$WailsExe = Join-Path $ToolsDir 'wails.exe'
New-Item -ItemType Directory -Force -Path $GoTmp, $GoCache | Out-Null

function Assert-Exit([string]$Step) {
    if ($LASTEXITCODE -ne 0) { throw "$Step 失败（退出码 $LASTEXITCODE）" }
}

foreach ($Tool in @('go', 'node', 'npm.cmd')) {
    if (-not (Get-Command $Tool -ErrorAction SilentlyContinue)) { throw "请先安装 $Tool，参见 desktop\README.md" }
}

if (-not (Test-Path -LiteralPath $WailsExe)) {
    Write-Host '正在准备桌面构建工具…' -ForegroundColor Cyan
    $PreviousGoBin = $env:GOBIN
    $PreviousTemp = $env:TEMP
    $PreviousTmp = $env:TMP
    $PreviousGoCache = $env:GOCACHE
    try {
        $env:GOBIN = $ToolsDir
        $env:TEMP = $GoTmp
        $env:TMP = $GoTmp
        $env:GOCACHE = $GoCache
        go install github.com/wailsapp/wails/v2/cmd/wails@v2.11.0
        Assert-Exit '构建工具安装'
    } finally {
        $env:GOBIN = $PreviousGoBin
        $env:TEMP = $PreviousTemp
        $env:TMP = $PreviousTmp
        $env:GOCACHE = $PreviousGoCache
    }
}

Push-Location (Join-Path $ProjectRoot 'web')
try {
    if (-not $SkipInstall) { npm.cmd ci; Assert-Exit '前端依赖安装' }
} finally { Pop-Location }

Push-Location $DesktopRoot
$PreviousTemp = $env:TEMP
$PreviousTmp = $env:TMP
$PreviousGoCache = $env:GOCACHE
try {
    $env:TEMP = $GoTmp
    $env:TMP = $GoTmp
    $env:GOCACHE = $GoCache
    # The launcher runs schema migrations from an embedded copy of
    # database/migrations, refreshed on every build.
    Write-Host '正在同步数据库迁移文件…' -ForegroundColor Cyan
    $MigrationsDir = Join-Path $DesktopRoot 'migrations'
    New-Item -ItemType Directory -Force -Path $MigrationsDir | Out-Null
    Get-ChildItem (Join-Path $ProjectRoot 'database\migrations') -Filter '*.sql' |
        Copy-Item -Destination $MigrationsDir -Force
    Write-Host '正在生成 Windows 桌面客户端…' -ForegroundColor Cyan
    & $WailsExe build -clean -trimpath -webview2 download -o WebsocketChat.exe
    Assert-Exit '桌面客户端构建'
    $Executable = Join-Path $DesktopRoot 'build\bin\WebsocketChat.exe'
    $Readme = Join-Path $DesktopRoot 'build\bin\使用说明.txt'
    $Text = @'
协作聊天 · Windows 桌面客户端

双击 WebsocketChat.exe 即可运行，无需安装 Go、Node.js 或浏览器扩展。
需要 Windows 10/11、Microsoft Edge WebView2 Runtime，以及本机安装的 PostgreSQL 12+
（或设置环境变量 IM_POSTGRES_BIN 指向其 bin 目录）。Redis 缺失时会自动下载便携版。

默认连接本机 http://127.0.0.1:8080。启动器会自动完成其余工作：启动私有 PostgreSQL
（端口 5434）、启动 Redis（端口 6380）、应用数据库结构并启动内置聊天服务（端口 8080）。
窗口顶部显示启动进度；就绪后即可登录。若端口上已有聊天服务在运行，则直接复用。

连接团队服务器：点击“服务器设置”，填写服务器地址并保存；远程地址不会启动本机服务。
高级设置中的网站地址需与服务器 FRONTEND_ORIGIN 相同，默认本机为 http://127.0.0.1:4173。

本机服务数据保存在 %APPDATA%\WebsocketChat（pgdata、redis-data、media、migrations、logs）；
不可写时使用程序旁的 WebsocketChat-data，请保留该目录，临时目录数据可能被系统清理。
登录凭据仅保留于进程内存，关闭软件后需要重新登录。
同事通讯录支持查找同事、按部门筛选、直接发消息和维护本人的工作资料。
待确认消息保存在本机，重新登录同一账号后可继续确认；退出登录会主动清除该账号待确认记录。
点击聊天图片或文件链接可打开系统“另存为”窗口。视频可直接播放。
关闭窗口即退出，由启动器拉起的 PostgreSQL 与 Redis 会一并停止；重复启动会唤起已有窗口。
'@
    [IO.File]::WriteAllText($Readme, $Text, (New-Object System.Text.UTF8Encoding($true)))
    $Archive = Join-Path $DesktopRoot 'build\bin\WebsocketChat-windows-amd64.zip'
    Compress-Archive -LiteralPath $Executable, $Readme -DestinationPath $Archive -Force
    Write-Host "已生成：$Executable" -ForegroundColor Green
    Write-Host "分发压缩包：$Archive" -ForegroundColor Green
} finally {
    $env:TEMP = $PreviousTemp
    $env:TMP = $PreviousTmp
    $env:GOCACHE = $PreviousGoCache
    Pop-Location
}
