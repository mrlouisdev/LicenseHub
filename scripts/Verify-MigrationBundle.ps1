[CmdletBinding()]
param(
    [Parameter(Mandatory)] [string]$Bundle
)

$ErrorActionPreference = 'Stop'
$bundlePath = [IO.Path]::GetFullPath($Bundle).TrimEnd([IO.Path]::DirectorySeparatorChar)
$prefix = $bundlePath + [IO.Path]::DirectorySeparatorChar
if (-not (Test-Path -LiteralPath $bundlePath -PathType Container)) { throw "Migration bundle not found: $bundlePath" }
$checksums = Join-Path $bundlePath 'migration-checksums.sha256'
$metadataPath = Join-Path $bundlePath 'migration-metadata.json'
$required = @(
    'deploy/docker-compose.yml', 'deploy/docker-compose.integrated.yml', 'deploy/images.lock',
    'deploy/lib.sh', 'deploy/new-env.sh', 'deploy/deploy.sh', 'deploy/backup.sh',
    'deploy/restore.sh', 'deploy/recover-host.sh', 'deploy/verify.sh', 'deploy/monitor.sh',
    'deploy/install-operations.sh', 'server/Dockerfile', 'server/go.mod',
    'migration-metadata.json', 'release-manifest.json'
)
foreach ($relative in $required) {
    if (-not (Test-Path -LiteralPath (Join-Path $bundlePath $relative) -PathType Leaf)) {
        throw "Required migration artifact is missing: $relative"
    }
}
if (-not (Test-Path -LiteralPath $checksums -PathType Leaf)) { throw 'migration-checksums.sha256 is missing' }

$seen = [Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
foreach ($line in Get-Content -LiteralPath $checksums) {
    if ($line -notmatch '^([a-f0-9]{64})  (.+)$') { throw "Invalid checksum line: $line" }
    $relative = $Matches[2]
    if (-not $seen.Add($relative)) { throw "Duplicate checksum entry: $relative" }
    $path = [IO.Path]::GetFullPath((Join-Path $bundlePath $relative))
    if (-not $path.StartsWith($prefix, [StringComparison]::OrdinalIgnoreCase)) { throw "Checksum path escapes bundle: $relative" }
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { throw "Checksummed file is missing: $relative" }
    $actual = (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actual -ne $Matches[1]) { throw "Checksum mismatch: $relative" }
}

$actualFiles = @(Get-ChildItem -LiteralPath $bundlePath -Recurse -File | ForEach-Object {
    [IO.Path]::GetRelativePath($bundlePath, $_.FullName).Replace('\', '/')
} | Where-Object { $_ -ne 'migration-checksums.sha256' })
$missingFromManifest = @($actualFiles | Where-Object { -not $seen.Contains($_) })
if ($missingFromManifest.Count) { throw "Unchecksummed files in migration bundle: $($missingFromManifest -join ', ')" }

$metadata = Get-Content -LiteralPath $metadataPath -Raw | ConvertFrom-Json
if ($metadata.format_version -ne 1 -or $metadata.deployment_status -ne 'staged_not_deployed') {
    throw 'Migration metadata is unsupported or already claims deployment'
}
Write-Output "MIGRATION_BUNDLE_OK $bundlePath FILES $($seen.Count) COMMIT $($metadata.source_commit) DIRTY $($metadata.source_worktree_dirty)"
