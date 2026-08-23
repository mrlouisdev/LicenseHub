[CmdletBinding()]
param(
    [string]$Runtime = 'win-x64',
    [string]$OutputRoot = (Join-Path $PSScriptRoot '..\artifacts'),
    [switch]$SkipBuild
)

$ErrorActionPreference = 'Stop'
$workspace = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..')).TrimEnd([IO.Path]::DirectorySeparatorChar)
$outputRootPath = [IO.Path]::GetFullPath($OutputRoot)
$workspacePrefix = $workspace + [IO.Path]::DirectorySeparatorChar
if (-not $outputRootPath.StartsWith($workspacePrefix, [StringComparison]::OrdinalIgnoreCase)) {
    throw 'OutputRoot must be inside the LicenseHub workspace.'
}
if ($Runtime -ne 'win-x64') { throw 'Only win-x64 is currently supported by the native client payload.' }

$native = Join-Path $workspace 'core\target\release\license_core.dll'
$importLibrary = Join-Path $workspace 'core\target\release\license_core.dll.lib'
$nodeRuntime = Join-Path $workspace 'bindings\node\node_modules\koffi'
foreach ($required in @($native, $nodeRuntime)) {
    if (-not (Test-Path -LiteralPath $required)) { throw "Required release payload is missing: $required" }
}

$stage = Join-Path $outputRootPath "licctl-portable-$Runtime"
$archive = Join-Path $outputRootPath "licctl-portable-$Runtime.zip"
if (Test-Path -LiteralPath $stage) { throw "Refusing to overwrite existing portable directory: $stage" }
if (Test-Path -LiteralPath $archive) { throw "Refusing to overwrite existing portable archive: $archive" }
New-Item -ItemType Directory -Path $stage -Force | Out-Null

if (-not $SkipBuild) {
    & dotnet publish (Join-Path $workspace 'cli\licctl\licctl.csproj') -c Release -r $Runtime --self-contained true -p:PublishSingleFile=true -p:DebugType=None -o $stage
    if ($LASTEXITCODE -ne 0) { throw 'licctl publish failed' }
} else {
    $existing = Join-Path $workspace "cli\licctl\dist\$Runtime\licctl.exe"
    if (-not (Test-Path -LiteralPath $existing -PathType Leaf)) { throw "Published licctl is missing: $existing" }
    Copy-Item -LiteralPath $existing -Destination $stage
}

function Copy-SourceTree([string]$Source, [string]$Destination, [string[]]$ExcludeDirectory = @()) {
    if (-not (Test-Path -LiteralPath $Source -PathType Container)) { throw "Payload directory is missing: $Source" }
    New-Item -ItemType Directory -Path $Destination -Force | Out-Null
    foreach ($file in Get-ChildItem -LiteralPath $Source -Recurse -File) {
        $relative = [IO.Path]::GetRelativePath($Source, $file.FullName)
        $segments = $relative -split '[\\/]'
        if (@($segments | Where-Object { $ExcludeDirectory -contains $_ }).Count) { continue }
        $target = Join-Path $Destination $relative
        New-Item -ItemType Directory -Path ([IO.Path]::GetDirectoryName($target)) -Force | Out-Null
        Copy-Item -LiteralPath $file.FullName -Destination $target
    }
}

Copy-SourceTree (Join-Path $workspace 'bindings\dotnet\src\LicenseHub.Licensing') (Join-Path $stage 'bindings\dotnet\src\LicenseHub.Licensing') @('bin', 'obj')
Copy-SourceTree (Join-Path $workspace 'bindings\python\licensehub_licensing') (Join-Path $stage 'bindings\python\licensehub_licensing') @('__pycache__')
Copy-SourceTree (Join-Path $workspace 'bindings\cpp\include') (Join-Path $stage 'bindings\cpp\include')
Copy-SourceTree (Join-Path $workspace 'bindings\node\src') (Join-Path $stage 'bindings\node\src')
Copy-SourceTree (Join-Path $workspace 'bindings\node\native') (Join-Path $stage 'bindings\node\native')
Copy-SourceTree $nodeRuntime (Join-Path $stage 'bindings\node\node_modules\koffi')
Copy-Item -LiteralPath (Join-Path $workspace 'bindings\node\package.json') -Destination (Join-Path $stage 'bindings\node\package.json')
Copy-Item -LiteralPath (Join-Path $workspace 'bindings\node\package-lock.json') -Destination (Join-Path $stage 'bindings\node\package-lock.json')

New-Item -ItemType Directory -Path (Join-Path $stage 'core\include'), (Join-Path $stage 'core\target\release') -Force | Out-Null
Copy-Item -LiteralPath (Join-Path $workspace 'core\include\license_core.h') -Destination (Join-Path $stage 'core\include\license_core.h')
Copy-Item -LiteralPath $native -Destination (Join-Path $stage 'core\target\release\license_core.dll')
# Never let a previously committed adapter payload override the DLL built for this release.
foreach ($adapterNative in @(
    (Join-Path $stage 'bindings\node\native\win-x64\license_core.dll'),
    (Join-Path $stage 'bindings\python\licensehub_licensing\_native\win-x64\license_core.dll')
)) {
    New-Item -ItemType Directory -Path ([IO.Path]::GetDirectoryName($adapterNative)) -Force | Out-Null
    Copy-Item -LiteralPath $native -Destination $adapterNative -Force
}
if (Test-Path -LiteralPath $importLibrary) {
    Copy-Item -LiteralPath $importLibrary -Destination (Join-Path $stage 'core\target\release\license_core.dll.lib')
}

$manifestPath = Join-Path $stage 'portable-manifest.json'
$files = @(Get-ChildItem -LiteralPath $stage -Recurse -File | Where-Object FullName -ne $manifestPath | ForEach-Object {
    [ordered]@{
        path = [IO.Path]::GetRelativePath($stage, $_.FullName).Replace('\', '/')
        bytes = $_.Length
        sha256 = (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
    }
} | Sort-Object path)
[ordered]@{
    format_version = 1
    runtime = $Runtime
    created_at_utc = (Get-Date).ToUniversalTime().ToString('o')
    files = $files
} | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath $manifestPath -Encoding utf8NoBOM

Compress-Archive -Path (Join-Path $stage '*') -DestinationPath $archive -CompressionLevel Optimal
$archiveHash = (Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash.ToLowerInvariant()
Write-Output "LICCTL_PORTABLE_READY $archive SHA256 $archiveHash FILES $($files.Count)"
