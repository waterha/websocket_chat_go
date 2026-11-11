param()
$ErrorActionPreference = 'Stop'
$ProjectRoot = Split-Path -Parent $PSScriptRoot
$Cache = Join-Path $ProjectRoot '.dev\runtime-downloads'
$Destination = Join-Path $ProjectRoot 'desktop\internal\local\runtimes'
$ManifestPath = Join-Path $PSScriptRoot 'desktop-runtime-sources.json'
$Sources = Get-Content -LiteralPath $ManifestPath -Raw | ConvertFrom-Json
New-Item -ItemType Directory -Force -Path $Cache, $Destination | Out-Null
Add-Type -AssemblyName System.IO.Compression
Add-Type -AssemblyName System.IO.Compression.FileSystem
$VcDirectory = & (Join-Path $PSScriptRoot 'prepare-desktop-vc-runtime.ps1') -Cache $Cache
$Manifest = @{}
foreach ($Kind in @('postgres', 'redis')) {
    $Spec = $Sources.$Kind
    $Archive = Join-Path $Cache $Spec.file
    if (-not (Test-Path -LiteralPath $Archive) -or (Get-FileHash -LiteralPath $Archive -Algorithm SHA256).Hash -ne $Spec.sha256) {
        Write-Host "正在下载固定版本的 $Kind 运行时（仅构建机需要联网）…" -ForegroundColor Cyan
        $Partial = "$Archive.part"
        Invoke-WebRequest -Uri $Spec.url -OutFile $Partial -UseBasicParsing -TimeoutSec 1200
        if ((Get-FileHash -LiteralPath $Partial -Algorithm SHA256).Hash -ne $Spec.sha256) { throw "$Kind 下载校验失败，未用于构建" }
        Move-Item -LiteralPath $Partial -Destination $Archive -Force
    }
    Write-Host "正在准备内置 $Kind 运行时…" -ForegroundColor Cyan
    $Output = Join-Path $Destination "$Kind.zip"
    $TemporaryOutput = "$Output.part"
    $Input = [IO.Compression.ZipFile]::OpenRead($Archive)
    $File = [IO.File]::Open($TemporaryOutput, [IO.FileMode]::Create)
    $Zip = [IO.Compression.ZipArchive]::new($File, [IO.Compression.ZipArchiveMode]::Create)
    try {
        foreach ($Entry in $Input.Entries) {
            $Name = $Entry.FullName.Replace('\', '/')
            if ($Kind -eq 'postgres') {
                # Keep the server/tools, DLLs, extension resources and upstream
                # license notices. pgAdmin, debug symbols and SDK are not needed.
                if ($Name -notmatch '^pgsql/(bin/|lib/|share/|doc/.*(licen[cs]e|copying|copyright)|[^/]*(licen[cs]e|copying|copyright)[^/]*$)') { continue }
                if ($Name.EndsWith('/') -or $Name.EndsWith('.pdb')) { continue }
                $Name = $Name.Substring(6)
            } else {
                if ($Name.EndsWith('/')) { continue }
                # The official Redis archive has a single versioned directory.
                $Prefix = ($Input.Entries | Where-Object { $_.FullName -match '(^|/)redis-server\.exe$' } | Select-Object -First 1).FullName
                if (-not $Prefix) { throw 'Redis归档缺少server' }
                $Prefix = $Prefix.Substring(0, $Prefix.Length - 'redis-server.exe'.Length)
                if (-not $Name.StartsWith($Prefix)) { continue }
                $Name = $Name.Substring($Prefix.Length)
            }
            $Target = $Zip.CreateEntry($Name, [IO.Compression.CompressionLevel]::Optimal)
            $Target.LastWriteTime = [DateTimeOffset]::new(2026, 1, 1, 0, 0, 0, [TimeSpan]::Zero)
            $Reader = $Entry.Open(); $Writer = $Target.Open()
            try { $Reader.CopyTo($Writer) } finally { $Writer.Dispose(); $Reader.Dispose() }
        }
        if ($Kind -eq 'postgres') {
            foreach ($Name in @('msvcp140.dll','vcruntime140.dll','vcruntime140_1.dll','license.rtf')) {
                $EntryName = if ($Name -eq 'license.rtf') { 'licenses/microsoft-vc-runtime.rtf' } else { 'bin/'+$Name }
                $Target = $Zip.CreateEntry($EntryName, [IO.Compression.CompressionLevel]::Optimal)
                $Target.LastWriteTime = [DateTimeOffset]::new(2026, 1, 1, 0, 0, 0, [TimeSpan]::Zero)
                $Reader = [IO.File]::OpenRead((Join-Path $VcDirectory $Name)); $Writer = $Target.Open()
                try { $Reader.CopyTo($Writer) } finally { $Writer.Dispose(); $Reader.Dispose() }
            }
        }
    } finally { $Zip.Dispose(); $File.Dispose(); $Input.Dispose() }
    Move-Item -LiteralPath $TemporaryOutput -Destination $Output -Force
    $Required = @($Spec.required)
    $Check = [IO.Compression.ZipFile]::OpenRead($Output)
    try { foreach ($Path in $Required) { if (-not $Check.GetEntry($Path)) { throw "内置 $Kind 缺少 $Path" } } } finally { $Check.Dispose() }
    $Manifest[$Kind] = @{ version=$Spec.version; archive="$Kind.zip"; sha256=(Get-FileHash -LiteralPath $Output -Algorithm SHA256).Hash.ToLowerInvariant(); required=$Required }
}
$Text = $Manifest | ConvertTo-Json -Depth 5
[IO.File]::WriteAllText((Join-Path $Destination 'manifest.json'), $Text, [Text.UTF8Encoding]::new($false))
Write-Host '便携数据库与Redis已准备，将嵌入单个EXE。' -ForegroundColor Green
