$ErrorActionPreference = 'Continue'
. (Join-Path $PSScriptRoot 'dev-env.ps1')

Write-Host ''
Write-Host '停止 IM 项目服务' -ForegroundColor Cyan
Write-Host '================' -ForegroundColor Cyan

$PgPort = [int](Get-EnvValue 'PGPORT' '5433')
$RedisPort = [int](Get-EnvValue 'REDIS_PORT' '6379')
$HttpAddr = Get-EnvValue 'HTTP_ADDR' '127.0.0.1:8080'
$BackendPort = 8080
if ($HttpAddr -match ':(\d+)\s*$') { $BackendPort = [int]$Matches[1] }
$FrontendPort = 4173
$WebUrl = Get-EnvValue 'FRONTEND_ORIGIN' 'http://127.0.0.1:4173'
if ($WebUrl -match ':(\d+)\s*$') { $FrontendPort = [int]$Matches[1] }

# Frontend and backend are started in their own windows; stop the listeners
# first so the dev servers shut down cleanly, then the compiled backend.
foreach ($Port in @($FrontendPort, $BackendPort)) {
    $ListenerPid = Get-ListenerPid $Port
    if ($ListenerPid -gt 0) {
        Stop-Process -Id $ListenerPid -Force -ErrorAction SilentlyContinue
        Write-Ok "端口 $Port 已停止（PID $ListenerPid）"
    } else {
        Write-Step "端口 $Port 未在运行"
    }
}
Get-Process im-server -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue

# PostgreSQL: use the private cluster only, never an unrelated installation.
$PgBin = Find-PgBin
if ($PgBin -and (Test-Path $PgData)) {
    if (Test-TcpPort $PgPort) {
        & (Join-Path $PgBin 'pg_ctl.exe') -D $PgData -m fast -w stop | Out-Null
        Write-Ok "PostgreSQL 已停止（端口 $PgPort）"
    } else {
        Write-Step "PostgreSQL 未在运行（端口 $PgPort）"
    }
}

# Redis: shut down only the instance on our configured port.
$RedisDir = Find-RedisServer
if ($RedisDir) {
    if (Test-RedisPong $RedisDir $RedisPort) {
        & (Join-Path $RedisDir 'redis-cli.exe') -h 127.0.0.1 -p $RedisPort shutdown nosave 2>$null | Out-Null
        Write-Ok "Redis 已停止（端口 $RedisPort）"
    } else {
        Write-Step "Redis 未在运行（端口 $RedisPort）"
    }
}

Write-Host ''
Write-Host '全部服务已停止。' -ForegroundColor Green
