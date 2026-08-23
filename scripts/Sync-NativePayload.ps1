[CmdletBinding()]
param([switch]$VerifyClean)

$ErrorActionPreference = 'Stop'
$workspace = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$native = Join-Path $workspace 'core\target\release\license_core.dll'
if (-not (Test-Path -LiteralPath $native -PathType Leaf)) { throw "Built native core is missing: $native" }

$targets = @(
    (Join-Path $workspace 'bindings\node\native\win-x64\license_core.dll'),
    (Join-Path $workspace 'bindings\python\licensehub_licensing\_native\win-x64\license_core.dll')
)
foreach ($target in $targets) {
    New-Item -ItemType Directory -Path ([IO.Path]::GetDirectoryName($target)) -Force | Out-Null
    Copy-Item -LiteralPath $native -Destination $target -Force
}

if ($VerifyClean) {
    & git -C $workspace diff --exit-code -- @($targets | ForEach-Object { [IO.Path]::GetRelativePath($workspace, $_) })
    if ($LASTEXITCODE -ne 0) { throw 'Committed native adapter payload differs from the pinned-toolchain build' }
}

$hash = (Get-FileHash -LiteralPath $native -Algorithm SHA256).Hash.ToLowerInvariant()
Write-Output "NATIVE_PAYLOAD_SYNCED SHA256 $hash TARGETS $($targets.Count)"
