# Shared helpers for the one-click development launcher.
# Dot-source from start.ps1 / stop.ps1. Everything local lives under .dev/,
# which is git-ignored, so the repository stays clean.

$Root = Split-Path -Parent $PSScriptRoot
$Dev = Join-Path $Root '.dev'
$Logs = Join-Path $Dev 'logs'
$BinDir = Join-Path $Dev 'bin'
$PgData = Join-Path $Dev 'pgdata'
$RedisRoot = Join-Path $Dev 'redis'
$RedisData = Join-Path $Dev 'redis-data'
$PgPassFile = Join-Path $Dev 'pgpass.txt'
$RedisZipUrl = 'https://github.com/redis-windows/redis-windows/releases/download/8.10.2/Redis-8.10.2-Windows-x64-cygwin.zip'

function Write-Step([string]$Message) { Write-Host "  $Message" -ForegroundColor DarkGray }
function Write-Ok([string]$Message) { Write-Host "  [OK] $Message" -ForegroundColor Green }
function Write-Note([string]$Message) { Write-Host "  [..] $Message" -ForegroundColor Yellow }

function Get-EnvValue {
    param([string]$Key, [string]$Default = '')
    $Path = Join-Path $Root '.env'
    if (-not (Test-Path $Path)) { return $Default }
    foreach ($Line in [IO.File]::ReadAllLines($Path)) {
        if ($Line -match ('^\s*' + [regex]::Escape($Key) + '\s*=(.*)$')) {
            $Value = $Matches[1].Trim()
            if ($Value) { return $Value }
            return $Default
        }
    }
    return $Default
}

function Test-TcpPort {
    param([int]$Port, [int]$TimeoutMs = 300)
    $Client = New-Object System.Net.Sockets.TcpClient
    try {
        $Task = $Client.ConnectAsync('127.0.0.1', $Port)
        if (-not $Task.Wait($TimeoutMs)) { return $false }
        return $Client.Connected
    } catch {
        return $false
    } finally {
        $Client.Close()
    }
}

function Wait-TcpPort {
    param([int]$Port, [int]$Seconds = 60, [string]$Label = '')
    for ($i = 0; $i -lt $Seconds; $i++) {
        if (Test-TcpPort $Port) { return $true }
        Start-Sleep -Seconds 1
    }
    return $false
}

function Test-HttpOk([string]$Url) {
    try {
        $Response = Invoke-WebRequest -Uri $Url -UseBasicParsing -TimeoutSec 3
        return ($Response.StatusCode -eq 200)
    } catch {
        return $false
    }
}

function Wait-HttpOk {
    param([string]$Url, [int]$Seconds = 60)
    for ($i = 0; $i -lt $Seconds; $i++) {
        if (Test-HttpOk $Url) { return $true }
        Start-Sleep -Seconds 1
    }
    return $false
}

function Get-ListenerPid([int]$Port) {
    $Connection = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($Connection) { return [int]$Connection.OwningProcess }
    return 0
}

function Find-PgBin {
    $Candidates = @()
    if ($env:IM_POSTGRES_BIN) { $Candidates += $env:IM_POSTGRES_BIN }
    $Candidates += 'D:\Postgresql\bin'
    $Candidates += (Get-ChildItem 'C:\Program Files\PostgreSQL' -Directory -ErrorAction SilentlyContinue | Sort-Object Name -Descending | ForEach-Object { Join-Path $_.FullName 'bin' })
    $PgCtl = Get-Command pg_ctl.exe -ErrorAction SilentlyContinue
    if ($PgCtl) { $Candidates += (Split-Path -Parent $PgCtl.Source) }
    foreach ($Candidate in $Candidates) {
        if ($Candidate -and (Test-Path (Join-Path $Candidate 'pg_ctl.exe'))) { return $Candidate }
    }
    return ''
}

function Find-RedisServer {
    if ($env:IM_REDIS_DIR -and (Test-Path (Join-Path $env:IM_REDIS_DIR 'redis-server.exe'))) { return $env:IM_REDIS_DIR }
    if (Test-Path $RedisRoot) {
        $Found = Get-ChildItem $RedisRoot -Recurse -Filter 'redis-server.exe' -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($Found) { return $Found.DirectoryName }
    }
    $Redis = Get-Command redis-server.exe -ErrorAction SilentlyContinue
    if ($Redis) { return (Split-Path -Parent $Redis.Source) }
    return ''
}

function Ensure-EnvFile {
    $Path = Join-Path $Root '.env'
    if (Test-Path $Path) { return }
    $Template = Get-Content (Join-Path $Root '.env.example') -Raw
    $Template = $Template -replace '(?m)^POSTGRES_PASSWORD=.*$', ('POSTGRES_PASSWORD=' + [guid]::NewGuid().ToString('N'))
    $Template = $Template -replace '(?m)^PGPORT=.*$', 'PGPORT=5433'
    $Template = $Template -replace '(?m)^POSTGRES_PORT=.*$', 'POSTGRES_PORT=5433'
    $Template = $Template -replace '(?m)^REDIS_ADDR=.*$', 'REDIS_ADDR=127.0.0.1:6379'
    $Template = $Template -replace '(?m)^REDIS_PORT=.*$', 'REDIS_PORT=6379'
    [IO.File]::WriteAllText($Path, $Template, (New-Object System.Text.UTF8Encoding($false)))
    Write-Ok '.env 已生成（本地数据库密码为随机值）'
}

function Ensure-PostgresCluster([string]$PgBin) {
    if (Test-Path (Join-Path $PgData 'PG_VERSION')) { return }
    New-Item -ItemType Directory -Force -Path $PgData, $Logs | Out-Null
    if (-not (Test-Path $PgPassFile)) {
        Set-Content -Path $PgPassFile -Value ([guid]::NewGuid().ToString('N')) -NoNewline -Encoding ascii
    }
    Write-Note '首次运行：初始化本地 PostgreSQL 数据目录（只在本机，不影响已安装的数据库）'
    & (Join-Path $PgBin 'initdb.exe') -D $PgData -U postgres "--pwfile=$PgPassFile" -E UTF8 -A scram-sha-256 --locale=C | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'initdb 失败，请查看上方输出。' }
    Write-Ok 'PostgreSQL 数据目录已初始化'
}

function Ensure-Database {
    param([string]$PgBin, [int]$Port, [string]$DbUser, [string]$DbName, [string]$DbPassword)
    $SuperPassword = (Get-Content $PgPassFile -Raw).Trim()
    $env:PGPASSWORD = $SuperPassword
    try {
        $RoleSql = "DO `$`$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = '$DbUser') THEN CREATE ROLE `"$DbUser`" LOGIN PASSWORD '$DbPassword'; ELSE ALTER ROLE `"$DbUser`" WITH LOGIN PASSWORD '$DbPassword'; END IF; END `$`$;"
        & (Join-Path $PgBin 'psql.exe') -h 127.0.0.1 -p $Port -U postgres -d postgres -v ON_ERROR_STOP=1 -q -c $RoleSql
        if ($LASTEXITCODE -ne 0) { throw '无法创建应用数据库账号，请检查本地 PostgreSQL 是否为本项目创建的那一个。' }

        $Existed = (& (Join-Path $PgBin 'psql.exe') -h 127.0.0.1 -p $Port -U postgres -d postgres -tA -c "SELECT 1 FROM pg_database WHERE datname = '$DbName'").Trim()
        if ($Existed -ne '1') {
            & (Join-Path $PgBin 'psql.exe') -h 127.0.0.1 -p $Port -U postgres -d postgres -v ON_ERROR_STOP=1 -q -c "CREATE DATABASE `"$DbName`" OWNER `"$DbUser`""
            if ($LASTEXITCODE -ne 0) { throw '无法创建应用数据库。' }
            Write-Ok "数据库 $DbName 已创建"
        }
        & (Join-Path $PgBin 'psql.exe') -h 127.0.0.1 -p $Port -U postgres -d postgres -v ON_ERROR_STOP=1 -q -c "ALTER DATABASE `"$DbName`" OWNER TO `"$DbUser`""
    } finally {
        Remove-Item Env:PGPASSWORD -ErrorAction SilentlyContinue
    }
}

function Start-PostgresIfNeeded {
    param([string]$PgBin, [int]$Port)
    if (Test-TcpPort $Port) { Write-Ok "PostgreSQL 已在运行（端口 $Port）"; return }
    Write-Note "启动本地 PostgreSQL（端口 $Port）"
    New-Item -ItemType Directory -Force -Path $Logs | Out-Null
    & (Join-Path $PgBin 'pg_ctl.exe') -D $PgData -l (Join-Path $Logs 'postgres.log') -o "-p $Port -h 127.0.0.1" -w start
    if ($LASTEXITCODE -ne 0 -or -not (Wait-TcpPort $Port 30)) {
        throw "PostgreSQL 启动失败，日志见 .dev\logs\postgres.log"
    }
    Write-Ok "PostgreSQL 已启动（端口 $Port）"
}

# redis-cli writes "Connection refused" to stderr when the server is down;
# with $ErrorActionPreference = 'Stop' that would abort the launcher, so probe
# under a local Continue preference.
function Test-RedisPong {
    param([string]$RedisDir, [int]$Port)
    $Previous = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try {
        $Ping = & (Join-Path $RedisDir 'redis-cli.exe') -h 127.0.0.1 -p $Port ping 2>$null
        return ($Ping -eq 'PONG')
    } finally {
        $ErrorActionPreference = $Previous
    }
}

function Ensure-Redis {
    param([int]$Port)
    $RedisDir = Find-RedisServer
    if (-not $RedisDir) {
        Write-Note '首次运行：下载便携版 Redis（约 15 MB）'
        New-Item -ItemType Directory -Force -Path $RedisRoot | Out-Null
        [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
        $Zip = Join-Path $Dev 'redis.zip'
        Invoke-WebRequest -Uri $RedisZipUrl -OutFile $Zip -UseBasicParsing
        Expand-Archive -Path $Zip -DestinationPath $RedisRoot -Force
        Remove-Item $Zip -Force
        $RedisDir = Find-RedisServer
        if (-not $RedisDir) { throw 'Redis 下载后仍未找到 redis-server.exe，请设置 IM_REDIS_DIR 指向 Redis 目录。' }
        Write-Ok 'Redis 已就绪'
    }
    if (Test-RedisPong $RedisDir $Port) { Write-Ok "Redis 已在运行（端口 $Port）"; return }
    Write-Note "启动 Redis（端口 $Port）"
    New-Item -ItemType Directory -Force -Path $RedisData | Out-Null
    Start-Process -FilePath (Join-Path $RedisDir 'redis-server.exe') `
        -ArgumentList '--port', $Port, '--bind', '127.0.0.1', '--dir', $RedisData, '--appendonly', 'no', '--save', '""' `
        -WorkingDirectory $RedisDir -WindowStyle Hidden | Out-Null
    for ($i = 0; $i -lt 20; $i++) {
        Start-Sleep -Milliseconds 500
        if (Test-RedisPong $RedisDir $Port) { Write-Ok "Redis 已启动（端口 $Port）"; return }
    }
    throw 'Redis 启动失败。'
}

function Ensure-FrontendDependencies {
    if (Test-Path (Join-Path $Root 'web\node_modules')) { return }
    Write-Note '首次运行：安装前端依赖'
    Push-Location (Join-Path $Root 'web')
    try {
        & npm.cmd ci --no-audit --no-fund 2>&1 | Out-Null
        if ($LASTEXITCODE -ne 0) {
            Write-Note '默认源失败，改用 npmmirror 镜像重试'
            & npm.cmd ci --no-audit --no-fund --registry=https://registry.npmmirror.com 2>&1 | Out-Null
        }
        if ($LASTEXITCODE -ne 0) { throw '前端依赖安装失败，请检查网络。' }
    } finally {
        Pop-Location
    }
    Write-Ok '前端依赖已安装'
}
