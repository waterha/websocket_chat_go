param(
    [switch]$NoBrowser,
    [switch]$SkipBuild
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'dev-env.ps1')

$StartedAt = Get-Date
Write-Host ''
Write-Host 'IM 项目一键启动' -ForegroundColor Cyan
Write-Host '================' -ForegroundColor Cyan

# ---- configuration ------------------------------------------------------
Ensure-EnvFile
$PgPort = [int](Get-EnvValue 'PGPORT' '5433')
$DbUser = Get-EnvValue 'PGUSER' 'websocket_chat_go'
$DbName = Get-EnvValue 'PGDATABASE' 'websocket_chat_go'
$DbPassword = Get-EnvValue 'POSTGRES_PASSWORD' ''
$RedisPort = [int](Get-EnvValue 'REDIS_PORT' '6379')
$RedisAddr = Get-EnvValue 'REDIS_ADDR' '127.0.0.1:6379'
$WebUrl = Get-EnvValue 'FRONTEND_ORIGIN' 'http://127.0.0.1:4173'

$HttpAddr = Get-EnvValue 'HTTP_ADDR' '127.0.0.1:8080'
$BackendPort = 8080
if ($HttpAddr -match ':(\d+)\s*$') { $BackendPort = [int]$Matches[1] }
$FrontendPort = 4173
if ($WebUrl -match ':(\d+)\s*$') { $FrontendPort = [int]$Matches[1] }

if (-not $DbPassword) { throw '.env 中缺少 POSTGRES_PASSWORD，请修复后重试。' }

# ---- PostgreSQL ---------------------------------------------------------
$PgBin = Find-PgBin
if (-not $PgBin) {
    throw '未找到 PostgreSQL。请安装 PostgreSQL 12+，或设置环境变量 IM_POSTGRES_BIN 指向其 bin 目录。'
}

Ensure-PostgresCluster $PgBin
Start-PostgresIfNeeded $PgBin $PgPort
Ensure-Database -PgBin $PgBin -Port $PgPort -DbUser $DbUser -DbName $DbName -DbPassword $DbPassword

# ---- Redis --------------------------------------------------------------
Ensure-Redis $RedisPort

# ---- backend ------------------------------------------------------------
New-Item -ItemType Directory -Force -Path $BinDir, $Logs | Out-Null
$BackendExe = Join-Path $BinDir 'im-server.exe'
$BackendOut = Join-Path $Logs 'backend.out.log'
$BackendErr = Join-Path $Logs 'backend.err.log'

if (Test-TcpPort $BackendPort) {
    if (Test-HttpOk "http://127.0.0.1:$BackendPort/healthz") {
        Write-Ok "后端已在运行（端口 $BackendPort）"
    } else {
        $ListenerPid = Get-ListenerPid $BackendPort
        throw "端口 $BackendPort 已被其他程序占用（PID $ListenerPid）。请关闭它，或修改 .env 中的 HTTP_ADDR 后重试。"
    }
} else {
    Write-Note '编译后端'
    Push-Location (Join-Path $Root 'backend')
    try {
        & go build -o $BackendExe ./cmd/server
        if ($LASTEXITCODE -ne 0) { throw '后端编译失败，请查看上方错误。' }
    } finally {
        Pop-Location
    }
    Remove-Item $BackendOut, $BackendErr -Force -ErrorAction SilentlyContinue
    $BackendCommand = "Set-Location -LiteralPath '$Root\backend'; `$Host.UI.RawUI.WindowTitle = 'IM 后端'; & '$BackendExe' 2>&1 | Tee-Object -FilePath '$BackendOut'"
    Start-Process -FilePath 'powershell.exe' -ArgumentList '-NoProfile', '-Command', $BackendCommand -WindowStyle Minimized | Out-Null
    if (-not (Wait-HttpOk "http://127.0.0.1:$BackendPort/healthz" 90)) {
        $Tail = Get-Content $BackendOut -Tail 20 -ErrorAction SilentlyContinue
        throw ("后端启动失败，日志见 .dev\logs\backend.out.log`n" + ($Tail -join "`n"))
    }
    Write-Ok "后端已启动（端口 $BackendPort）"
}

# ---- frontend -----------------------------------------------------------
Ensure-FrontendDependencies

if (Test-TcpPort $FrontendPort) {
    if (Test-HttpOk $WebUrl) {
        Write-Ok "前端已在运行（端口 $FrontendPort）"
    } else {
        $ListenerPid = Get-ListenerPid $FrontendPort
        throw "端口 $FrontendPort 已被其他程序占用（PID $ListenerPid）。请关闭它，或修改 .env 中的 FRONTEND_ORIGIN 后重试。"
    }
} else {
    Write-Note '启动前端开发服务器'
    $FrontendCommand = "Set-Location -LiteralPath '$Root\web'; `$Host.UI.RawUI.WindowTitle = 'IM 前端'; npm run dev 2>&1 | Tee-Object -FilePath '$Logs\frontend.log'"
    Start-Process -FilePath 'powershell.exe' -ArgumentList '-NoProfile', '-Command', $FrontendCommand -WindowStyle Minimized | Out-Null
    if (-not (Wait-HttpOk $WebUrl 90)) {
        $Tail = Get-Content (Join-Path $Logs 'frontend.log') -Tail 20 -ErrorAction SilentlyContinue
        throw ("前端启动失败，日志见 .dev\logs\frontend.log`n" + ($Tail -join "`n"))
    }
    Write-Ok "前端已启动（端口 $FrontendPort）"
}

# ---- done ---------------------------------------------------------------
$Elapsed = [int]((Get-Date) - $StartedAt).TotalSeconds
Write-Host ''
Write-Host "启动完成，用时 $Elapsed 秒" -ForegroundColor Green
Write-Host ''
Write-Host "  前端页面    $WebUrl" -ForegroundColor Cyan
Write-Host "  后端接口    http://127.0.0.1:$BackendPort" -ForegroundColor DarkCyan
Write-Host "  健康检查    http://127.0.0.1:$BackendPort/healthz" -ForegroundColor DarkCyan
Write-Host "  数据库      PostgreSQL 127.0.0.1:$PgPort / $DbName" -ForegroundColor DarkCyan
Write-Host "  缓存        $RedisAddr" -ForegroundColor DarkCyan
Write-Host ''
Write-Host '  关闭服务请双击 停止项目.bat；窗口会自动：后端、前端（最小化，日志在里面）。' -ForegroundColor DarkGray

if (-not $NoBrowser) { Start-Process $WebUrl }
