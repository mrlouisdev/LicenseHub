[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
$workspace = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$native = Join-Path $workspace 'core\target\release\license_core.dll'
if (-not (Test-Path -LiteralPath $native -PathType Leaf)) { throw "Built native core is missing: $native" }

$targets = @(
    (Join-Path $workspace 'bindings\node\native\win-x64\license_core.dll'),
    (Join-Path $workspace 'bindings\python\licensehub_licensing\_native\win-x64\license_core.dll')
)
$hash = (Get-FileHash -LiteralPath $native -Algorithm SHA256).Hash.ToLowerInvariant()
foreach ($target in $targets) {
    New-Item -ItemType Directory -Path ([IO.Path]::GetDirectoryName($target)) -Force | Out-Null
    Copy-Item -LiteralPath $native -Destination $target -Force
    $targetHash = (Get-FileHash -LiteralPath $target -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($targetHash -ne $hash) { throw "Native payload hash mismatch after copy: $target" }
}

Write-Output "NATIVE_PAYLOAD_SYNCED SHA256 $hash TARGETS $($targets.Count)"
