[CmdletBinding()]
param(
    [string]$Manifest = (Join-Path $PSScriptRoot '..\release-manifest.json')
)

$ErrorActionPreference = 'Stop'
$manifestPath = [IO.Path]::GetFullPath($Manifest)
$workspace = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$workspacePrefix = $workspace.TrimEnd([IO.Path]::DirectorySeparatorChar) + [IO.Path]::DirectorySeparatorChar

if (-not (Test-Path -LiteralPath $manifestPath -PathType Leaf)) {
    throw "Release manifest not found: $manifestPath"
}

$release = Get-Content -Raw -LiteralPath $manifestPath | ConvertFrom-Json
$failures = @()
foreach ($artifact in $release.artifacts) {
    $path = [IO.Path]::GetFullPath((Join-Path $workspace ([string]$artifact.path)))
    if (-not $path.StartsWith($workspacePrefix, [StringComparison]::OrdinalIgnoreCase)) {
        $failures += "path escapes workspace: $($artifact.path)"
        continue
    }
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        $failures += "missing: $($artifact.path)"
        continue
    }
    $file = Get-Item -LiteralPath $path
    if ($file.Length -ne [long]$artifact.bytes) {
        $failures += "size mismatch: $($artifact.path)"
    }
    $hash = (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($hash -ne ([string]$artifact.sha256).ToLowerInvariant()) {
        $failures += "hash mismatch: $($artifact.path)"
    }
}

if ($failures.Count) {
    throw "Release verification failed: $($failures -join '; ')"
}

Write-Output "RELEASE_MANIFEST_OK $($release.release) $($release.artifacts.Count)_ARTIFACTS"
