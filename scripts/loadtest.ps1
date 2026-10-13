param(
    [int]$Members = 1000,
    [string]$Rates = '1,5,10',
    [string]$Idle = '30s',
    [string]$Duration = '20s',
    [string]$Drain = '60s',
    [string]$OutputDir = (Join-Path $env:TEMP ('im-ws-load-' + [guid]::NewGuid().ToString('N')))
)

$ErrorActionPreference = 'Stop'
if (-not $env:IM_TEST_DATABASE_URL -or -not $env:IM_TEST_REDIS_ADDR) {
    throw 'Set IM_TEST_DATABASE_URL and IM_TEST_REDIS_ADDR to isolated test dependencies.'
}
$repo = Split-Path $PSScriptRoot -Parent
$directory = [System.IO.Path]::GetFullPath($OutputDir)
New-Item -ItemType Directory -Path $directory -Force | Out-Null
$executable = Join-Path $directory 'loadtest.exe'
$fixture = Join-Path $directory 'credentials.json'
$result = Join-Path $directory 'result.json'
$serverMetrics = Join-Path $directory 'server-metrics.jsonl'
$processMetrics = Join-Path $directory 'process-metrics.csv'
if (Test-Path -LiteralPath $fixture) { throw 'Output directory already contains a credential file.' }
Push-Location (Join-Path $repo 'backend')
try {
    & go build -o $executable ./cmd/loadtest
    if ($LASTEXITCODE -ne 0) { throw 'Could not build load test tool.' }
} finally { Pop-Location }

$server = $null
$client = $null
$logicalProcessors = [Environment]::ProcessorCount
$samples = [System.Collections.Generic.List[object]]::new()
try {
    $serverArgs = @('-mode', 'server', '-members', $Members, '-fixture', "`"$fixture`"", '-metrics', "`"$serverMetrics`"", '-migrations', "`"$(Join-Path $repo 'database/migrations')`"")
    $server = Start-Process -FilePath $executable -ArgumentList $serverArgs -PassThru -WindowStyle Hidden -RedirectStandardOutput (Join-Path $directory 'server.stdout.log') -RedirectStandardError (Join-Path $directory 'server.stderr.log')
    $deadline = [DateTime]::UtcNow.AddSeconds(45)
    while (-not (Test-Path -LiteralPath $fixture)) {
        if ($server.HasExited -or [DateTime]::UtcNow -gt $deadline) {
            throw "Test server did not start. See $directory/server.stderr.log"
        }
        Start-Sleep -Milliseconds 200
        $server.Refresh()
    }
    $clientArgs = @('-mode', 'client', '-members', $Members, '-fixture', "`"$fixture`"", '-output', "`"$result`"", '-rates', $Rates, '-idle', $Idle, '-duration', $Duration, '-drain', $Drain)
    $client = Start-Process -FilePath $executable -ArgumentList $clientArgs -PassThru -WindowStyle Hidden -RedirectStandardOutput (Join-Path $directory 'client.stdout.log') -RedirectStandardError (Join-Path $directory 'client.stderr.log')
    $previousTime = [DateTime]::UtcNow
    $previousServer = 0.0
    $previousClient = 0.0
    $first = $true
    while (-not $client.HasExited) {
        $server.Refresh()
        $client.Refresh()
        if ($server.HasExited -or $client.HasExited) { break }
        $now = [DateTime]::UtcNow
        $serverCpu = $server.TotalProcessorTime.TotalSeconds
        $clientCpu = $client.TotalProcessorTime.TotalSeconds
        $seconds = ($now - $previousTime).TotalSeconds
        $serverPercent = if ($first) { 0.0 } else { 100 * ($serverCpu - $previousServer) / $seconds / $logicalProcessors }
        $clientPercent = if ($first) { 0.0 } else { 100 * ($clientCpu - $previousClient) / $seconds / $logicalProcessors }
        $samples.Add([pscustomobject]@{
            time = $now.ToString('o')
            server_cpu_percent = [Math]::Round($serverPercent, 2)
            client_cpu_percent = [Math]::Round($clientPercent, 2)
            server_working_set_mib = [Math]::Round($server.WorkingSet64 / 1MB, 2)
            client_working_set_mib = [Math]::Round($client.WorkingSet64 / 1MB, 2)
            server_private_mib = [Math]::Round($server.PrivateMemorySize64 / 1MB, 2)
            client_private_mib = [Math]::Round($client.PrivateMemorySize64 / 1MB, 2)
        })
        $previousTime = $now
        $previousServer = $serverCpu
        $previousClient = $clientCpu
        $first = $false
        Start-Sleep -Seconds 1
        $client.Refresh()
    }
    $client.WaitForExit()
    $samples | Export-Csv -LiteralPath $processMetrics -NoTypeInformation -Encoding utf8
    [pscustomobject]@{
        recorded_at = [DateTime]::UtcNow.ToString('o')
        os = [Environment]::OSVersion.VersionString
        logical_processors = $logicalProcessors
        cpu = (Get-CimInstance Win32_Processor | Select-Object -ExpandProperty Name)
        physical_memory_bytes = (Get-CimInstance Win32_ComputerSystem).TotalPhysicalMemory
        client_exit_code = $client.ExitCode
    } | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $directory 'environment.json') -Encoding utf8
    Write-Output "Results: $directory"
    if (Test-Path -LiteralPath $result) { Get-Content -LiteralPath $result }
    if ($client.ExitCode -ne 0) {
        Get-Content -LiteralPath (Join-Path $directory 'client.stderr.log')
        throw 'Load criteria failed. Result files are retained.'
    }
} finally {
    if (Test-Path -LiteralPath $fixture) {
        try {
            $data = Get-Content -LiteralPath $fixture -Raw | ConvertFrom-Json
            Invoke-WebRequest -Uri ($data.URL + '/_loadtest/stop') -Method Post -Headers @{ 'X-Loadtest-Token' = $data.AdminToken } -TimeoutSec 5 | Out-Null
        } catch { Write-Warning 'Graceful test-server shutdown failed.' }
    }
    foreach ($process in @($client, $server)) {
        if ($null -eq $process) { continue }
        $process.Refresh()
        if (-not $process.HasExited -and -not $process.WaitForExit(15000)) {
            $live = Get-Process -Id $process.Id -ErrorAction SilentlyContinue
            if ($live -and [System.IO.Path]::GetFullPath($live.Path) -eq $executable) {
                Stop-Process -Id $process.Id
            }
        }
    }
    if (Test-Path -LiteralPath $fixture) { Remove-Item -LiteralPath $fixture }
}
