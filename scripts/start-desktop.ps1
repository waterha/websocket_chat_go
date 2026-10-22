param([switch]$LocalServer, [switch]$Rebuild)

$ErrorActionPreference = 'Stop'
$ProjectRoot = Split-Path -Parent $PSScriptRoot
$Executable = Join-Path $ProjectRoot 'desktop\build\bin\WebsocketChat.exe'
if ($Rebuild -or -not (Test-Path -LiteralPath $Executable)) {
    & (Join-Path $PSScriptRoot 'build-desktop.ps1')
}
if ($LocalServer) {
    & (Join-Path $PSScriptRoot 'start.ps1') -NoBrowser -NoFrontend
    . (Join-Path $PSScriptRoot 'dev-env.ps1')
    # The development launcher may choose another free backend port.
    $BackendAddress = Get-EnvValue 'HTTP_ADDR' '127.0.0.1:8080'
    $PreviousServer = $env:IM_DESKTOP_SERVER
    $PreviousOrigin = $env:IM_DESKTOP_ORIGIN
    try {
        $env:IM_DESKTOP_SERVER = "http://$BackendAddress"
        $env:IM_DESKTOP_ORIGIN = Get-EnvValue 'FRONTEND_ORIGIN' 'http://127.0.0.1:4173'
        Start-Process -FilePath $Executable -WorkingDirectory $ProjectRoot
    } finally {
        $env:IM_DESKTOP_SERVER = $PreviousServer
        $env:IM_DESKTOP_ORIGIN = $PreviousOrigin
    }
} else {
    Start-Process -FilePath $Executable -WorkingDirectory $ProjectRoot
}
