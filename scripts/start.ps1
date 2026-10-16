param(
    [switch]$NoNewWindows
)

$ErrorActionPreference = 'Stop'
$Root = Split-Path -Parent $PSScriptRoot

$PostgresBin = if ($env:IM_POSTGRES_BIN) { $env:IM_POSTGRES_BIN } else { 'C:\Sofware\pgsql-portable\pgsql\bin' }
$PostgresData = if ($env:IM_POSTGRES_DATA) { $env:IM_POSTGRES_DATA } else { 'C:\Sofware\pgsql-portable\data' }
$RedisDir = if ($env:IM_REDIS_DIR) { $env:IM_REDIS_DIR } else { 'C:\Sofware\Redis-8.8.1-Windows-x64-cygwin' }

function Test-ListeningPort([int]$Port) {
    return $null -ne (Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue)
}

function Wait-ForPort([int]$Port, [int]$Seconds = 20) {
    for ($index = 0; $index -lt $Seconds; $index++) {
        if (Test-ListeningPort $Port) { return }
        Start-Sleep -Seconds 1
    }
    throw "端口 $Port 未启动，请检查对应日志。"
}

function Start-Terminal([string]$Title, [string]$WorkingDirectory, [string]$Command) {
    if ($NoNewWindows) {
        Start-Process -FilePath 'pwsh.exe' -ArgumentList '-NoExit', '-Command', "Set-Location -LiteralPath '$WorkingDirectory'; `$Host.UI.RawUI.WindowTitle = '$Title'; $Command" -WorkingDirectory $WorkingDirectory
        return
    }
    Start-Process -FilePath 'pwsh.exe' -ArgumentList '-NoExit', '-Command', "Set-Location -LiteralPath '$WorkingDirectory'; `$Host.UI.RawUI.WindowTitle = '$Title'; $Command" -WorkingDirectory $WorkingDirectory
}

if (-not (Test-ListeningPort 55433)) {
    & (Join-Path $PostgresBin 'pg_ctl.exe') -D $PostgresData -l (Join-Path $Root 'postgres.log') -o '-p 55433 -h 127.0.0.1' start | Out-Host
    Wait-ForPort 55433
    Write-Host 'PostgreSQL 已启动（55433）' -ForegroundColor Green
} else {
    Write-Host 'PostgreSQL 已在运行（55433）' -ForegroundColor DarkGray
}

$redisPing = & (Join-Path $RedisDir 'redis-cli.exe') -h 127.0.0.1 -p 56379 ping 2>$null
if ($redisPing -ne 'PONG') {
    Start-Process -FilePath (Join-Path $RedisDir 'redis-server.exe') -ArgumentList 'redis.conf', '--port', '56379' -WorkingDirectory $RedisDir -WindowStyle Hidden | Out-Null
    Wait-ForPort 56379
    Write-Host 'Redis 已启动（56379）' -ForegroundColor Green
} else {
    Write-Host 'Redis 已在运行（56379）' -ForegroundColor DarkGray
}

if (-not (Test-ListeningPort 8080)) {
    Start-Terminal 'IM 后端' (Join-Path $Root 'backend') 'go run ./cmd/server'
    Write-Host '后端启动中（8080）' -ForegroundColor Green
} else {
    Write-Host '后端已在运行（8080）' -ForegroundColor DarkGray
}

if (-not (Test-ListeningPort 4173)) {
    Start-Terminal 'IM 前端' (Join-Path $Root 'web') 'npm run dev'
    Write-Host '前端启动中（4173）' -ForegroundColor Green
} else {
    Write-Host '前端已在运行（4173）' -ForegroundColor DarkGray
}

Write-Host ''
Write-Host '前端地址：http://127.0.0.1:4173' -ForegroundColor Cyan
Write-Host '后端健康检查：http://127.0.0.1:8080/healthz' -ForegroundColor Cyan
