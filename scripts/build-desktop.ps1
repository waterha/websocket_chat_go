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
    Write-Host '正在生成 Windows 桌面客户端…' -ForegroundColor Cyan
    & $WailsExe build -clean -trimpath -webview2 download -o WebsocketChat.exe
    Assert-Exit '桌面客户端构建'
    $Executable = Join-Path $DesktopRoot 'build\bin\WebsocketChat.exe'
    $Readme = Join-Path $DesktopRoot 'build\bin\使用说明.txt'
    $Text = @'
协作聊天 · Windows 桌面客户端

双击 WebsocketChat.exe 即可运行，无需安装 Go、Node.js、数据库或浏览器扩展。
需要 Windows 10/11 及 Microsoft Edge WebView2 Runtime；缺少时启动器会引导安装。

首次使用：点击窗口上方“服务器设置”，填写团队服务器地址，测试连接后保存。
高级设置中的网站地址需与服务器 FRONTEND_ORIGIN 相同，默认本机为 http://127.0.0.1:4173。
默认连接 http://127.0.0.1:8080。聊天服务须已启动；客户端不内置数据库。

设置和 WebView 数据保存在 %APPDATA%\WebsocketChat，登录凭据仅保留于进程内存，关闭软件后需要重新登录。
待确认消息保存在本机，重新登录同一账号后可继续确认；退出登录会主动清除该账号待确认记录。
点击聊天图片或文件链接可打开系统“另存为”窗口。视频可直接播放。
关闭窗口即退出；重复启动会唤起已有窗口。
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
