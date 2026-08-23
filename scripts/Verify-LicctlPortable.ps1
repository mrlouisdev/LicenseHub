[CmdletBinding()]
param(
    [Parameter(Mandatory)] [string]$Archive
)

$ErrorActionPreference = 'Stop'
$archivePath = [IO.Path]::GetFullPath($Archive)
if (-not (Test-Path -LiteralPath $archivePath -PathType Leaf)) { throw "Portable archive not found: $archivePath" }
$temporary = Join-Path ([IO.Path]::GetTempPath()) ('licctl-portable-verify-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $temporary | Out-Null
try {
    Expand-Archive -LiteralPath $archivePath -DestinationPath $temporary
    $manifestPath = Join-Path $temporary 'portable-manifest.json'
    $executable = Join-Path $temporary 'licctl.exe'
    foreach ($required in @($manifestPath, $executable, (Join-Path $temporary 'core\include\license_core.h'), (Join-Path $temporary 'core\target\release\license_core.dll'))) {
        if (-not (Test-Path -LiteralPath $required -PathType Leaf)) { throw "Portable payload is incomplete: $required" }
    }
    $manifest = Get-Content -LiteralPath $manifestPath -Raw | ConvertFrom-Json
    if ($manifest.format_version -ne 1 -or $manifest.runtime -ne 'win-x64') { throw 'Unsupported portable manifest' }
    foreach ($file in $manifest.files) {
        $path = [IO.Path]::GetFullPath((Join-Path $temporary ([string]$file.path)))
        $prefix = $temporary.TrimEnd([IO.Path]::DirectorySeparatorChar) + [IO.Path]::DirectorySeparatorChar
        if (-not $path.StartsWith($prefix, [StringComparison]::OrdinalIgnoreCase)) { throw "Manifest path escapes package: $($file.path)" }
        if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { throw "Manifest file is missing: $($file.path)" }
        $item = Get-Item -LiteralPath $path
        if ($item.Length -ne [long]$file.bytes) { throw "Size mismatch: $($file.path)" }
        $hash = (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant()
        if ($hash -ne ([string]$file.sha256).ToLowerInvariant()) { throw "Hash mismatch: $($file.path)" }
    }

    $kit = Join-Path $temporary 'generated-kit'
    $publicKey = [Convert]::ToBase64String([byte[]](0..31))
    & $executable init --product portable-fixture --server http://127.0.0.1:19001 --public-key "fixture=$publicKey" --out $kit
    if ($LASTEXITCODE -ne 0) { throw 'Portable licctl init failed outside the source checkout' }
    & $executable doctor --kit $kit --json | Out-Host
    if ($LASTEXITCODE -ne 0) { throw 'Generated portable kit failed doctor' }
    foreach ($payload in @('bindings\dotnet', 'bindings\node', 'bindings\python', 'bindings\cpp', 'core\include\license_core.h', 'core\target\release\license_core.dll')) {
        if (-not (Test-Path -LiteralPath (Join-Path $kit $payload))) { throw "Generated kit is missing: $payload" }
    }

    $workspace = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
    foreach ($stack in @('dotnet', 'electron', 'python', 'cpp')) {
        $project = Join-Path $temporary "fixture-$stack"
        Copy-Item -LiteralPath (Join-Path $workspace "tests\integration\fixtures\$stack") -Destination $project -Recurse
        $before = @(Get-ChildItem -LiteralPath $project -Recurse -File | Sort-Object FullName | ForEach-Object {
            "$([IO.Path]::GetRelativePath($project, $_.FullName)):$((Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash)"
        })
        & $executable add --project $project --kit $kit
        if ($LASTEXITCODE -ne 0) { throw "$stack add failed from the portable distribution" }
        & $executable doctor --project $project --json | Out-Host
        if ($LASTEXITCODE -ne 0) { throw "$stack doctor failed from the portable distribution" }
        & $executable remove --project $project
        if ($LASTEXITCODE -ne 0) { throw "$stack remove failed from the portable distribution" }
        $after = @(Get-ChildItem -LiteralPath $project -Recurse -File | Where-Object { $_.FullName -notmatch '[\\/](bin|obj)[\\/]' } | Sort-Object FullName | ForEach-Object {
            "$([IO.Path]::GetRelativePath($project, $_.FullName)):$((Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash)"
        })
        if (Compare-Object $before $after) { throw "$stack remove did not restore a clean fixture" }
    }
    Write-Output "LICCTL_PORTABLE_OK $archivePath FILES $($manifest.files.Count)"
} finally {
    if (Test-Path -LiteralPath $temporary) { Remove-Item -LiteralPath $temporary -Recurse -Force }
}
