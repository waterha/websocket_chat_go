param([Parameter(Mandatory=$true)][string]$Cache)
$ErrorActionPreference = 'Stop'
$Archive = Join-Path $Cache 'vc_redist.x64.exe'
$Expected = 'cc0ff0eb1dc3f5188ae6300faef32bf5beeba4bdd6e8e445a9184072096b713b'
if (-not (Test-Path -LiteralPath $Archive) -or (Get-FileHash $Archive).Hash -ne $Expected) {
    Invoke-WebRequest 'https://aka.ms/vs/17/release/vc_redist.x64.exe' -OutFile "$Archive.part" -UseBasicParsing -TimeoutSec 600
    if ((Get-FileHash "$Archive.part").Hash -ne $Expected) { throw '微软VC运行库版本已变化，请核实新版本并更新固定SHA256，禁止静默使用' }
    Move-Item -LiteralPath "$Archive.part" -Destination $Archive -Force
}
$Signature = Get-AuthenticodeSignature -LiteralPath $Archive
if ($Signature.Status -ne 'Valid' -or $Signature.SignerCertificate.Subject -notmatch 'O=Microsoft Corporation') { throw '微软VC运行库签名校验失败' }
$Output = Join-Path $Cache 'vc-app-local'
$Required = @('msvcp140.dll','vcruntime140.dll','vcruntime140_1.dll')
$Ready = $true
foreach ($Name in $Required + @('license.rtf')) { if (-not (Test-Path (Join-Path $Output $Name))) { $Ready = $false } }
if ($Ready) { return $Output }
$Work = Join-Path $Cache 'vc-extracted'
New-Item -ItemType Directory -Force $Output, $Work | Out-Null
# Microsoft Burn bundles contain standard CAB containers. Extract them without
# executing the installer, elevating, or installing anything on the build PC.
$Bytes = [IO.File]::ReadAllBytes($Archive)
$Offset = 0; $Index = 0
while ($Offset -lt $Bytes.Length) {
    $Offset = [Array]::IndexOf($Bytes, [byte]77, $Offset)
    if ($Offset -lt 0) { break }
    if ($Offset + 36 -lt $Bytes.Length -and $Bytes[$Offset+1] -eq 83 -and $Bytes[$Offset+2] -eq 67 -and $Bytes[$Offset+3] -eq 70) {
        $Reserved = [BitConverter]::ToUInt32($Bytes, $Offset+4)
        $Size = [BitConverter]::ToUInt32($Bytes, $Offset+8)
        if ($Reserved -eq 0 -and $Size -gt 36 -and $Size -le $Bytes.Length-$Offset) {
            $Cab = Join-Path $Work "container-$Index.cab"
            $Stream = [IO.File]::Create($Cab)
            try { $Stream.Write($Bytes, $Offset, $Size) } finally { $Stream.Dispose() }
            $Dir = Join-Path $Work "container-$Index"
            New-Item -ItemType Directory -Force $Dir | Out-Null
            & expand.exe -F:* $Cab $Dir | Out-Null
            if ($LASTEXITCODE -ne 0) { throw '微软VC运行库容器提取失败' }
            $Index++
        }
    }
    $Offset++
}
[xml]$Manifest = Get-Content (Join-Path $Work 'container-0\0') -Raw
$Payload = $Manifest.SelectNodes('//*[local-name()="Payload"]') | Where-Object { $_.FilePath -eq 'packages\vcRuntimeMinimum_amd64\cab1.cab' } | Select-Object -First 1
if (-not $Payload) { throw '固定VC运行库缺少x64 CAB映射' }
$DllDir = Join-Path $Work 'x64-dlls'
New-Item -ItemType Directory -Force $DllDir | Out-Null
& expand.exe -F:* (Join-Path $Work ('container-1\'+$Payload.SourcePath)) $DllDir | Out-Null
if ($LASTEXITCODE -ne 0) { throw '微软VC x64 DLL提取失败' }
foreach ($Name in $Required) {
    $Source = Join-Path $DllDir ($Name+'_amd64')
    if (-not (Test-Path $Source)) { throw "VC运行库缺少 $Name" }
    Copy-Item -LiteralPath $Source -Destination (Join-Path $Output $Name) -Force
}
$License = $Manifest.SelectNodes('//*[local-name()="Payload"]') | Where-Object { $_.FilePath -eq 'license.rtf' } | Select-Object -First 1
Copy-Item -LiteralPath (Join-Path $Work ('container-0\'+$License.SourcePath)) -Destination (Join-Path $Output 'license.rtf') -Force
return $Output
