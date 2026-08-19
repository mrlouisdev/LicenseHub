[CmdletBinding(SupportsShouldProcess = $true)]
param(
    [Parameter(Mandatory)] [string]$Destination,
    [string]$ComposeFile = (Join-Path $PSScriptRoot '..\deploy\docker-compose.yml'),
    [switch]$DryRun
)

$ErrorActionPreference = 'Stop'
$destinationPath = [IO.Path]::GetFullPath($Destination)
$workspace = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..')).TrimEnd([IO.Path]::DirectorySeparatorChar)
$workspacePrefix = $workspace + [IO.Path]::DirectorySeparatorChar

if ($destinationPath.StartsWith($workspacePrefix, [StringComparison]::OrdinalIgnoreCase)) {
    throw 'Migration destination must be outside the workspace.'
}
if (Test-Path -LiteralPath $destinationPath) {
    if ((Get-ChildItem -LiteralPath $destinationPath -Force | Measure-Object).Count -gt 0) {
        throw "Destination must be empty: $destinationPath"
    }
}

$plan = [ordered]@{
    operation = 'stage-vps-migration'
    destination = $destinationPath
    contents = @('database backup', 'Compose/Caddy config', 'server build context', 'migration runbook', 'SHA-256 manifest')
    server_configuration = 'secrets must be provisioned independently on the destination host'
}
if ($DryRun) { $plan | ConvertTo-Json; return }

if ($PSCmdlet.ShouldProcess($destinationPath, 'Create VPS migration staging directory')) {
    New-Item -ItemType Directory -Path $destinationPath -Force | Out-Null
    $backupRoot = Join-Path $destinationPath 'backup'
    & (Join-Path $PSScriptRoot 'Backup-LicenseHub.ps1') -ComposeFile $ComposeFile -OutputRoot $backupRoot | Out-Host

    $deployTarget = Join-Path $destinationPath 'deploy'
    New-Item -ItemType Directory -Path $deployTarget -Force | Out-Null
    Copy-Item -LiteralPath (Join-Path $workspace 'deploy\docker-compose.yml') -Destination $deployTarget
    Copy-Item -LiteralPath (Join-Path $workspace 'deploy\Caddyfile') -Destination $deployTarget
    Copy-Item -LiteralPath (Join-Path $workspace 'deploy\.env.example') -Destination $deployTarget
    Copy-Item -LiteralPath (Join-Path $workspace 'docs\vps-migration.md') -Destination $destinationPath

    # Compose builds ../server relative to deploy/docker-compose.yml. Stage the
    # exact source inputs needed by server/Dockerfile so this bundle can build
    # on a clean VPS without relying on the original workspace. Deliberately do
    # not copy local binaries, node_modules, .env files, or other build output.
    $serverTarget = Join-Path $destinationPath 'server'
    New-Item -ItemType Directory -Path $serverTarget -Force | Out-Null
    $serverFiles = @('go.mod', 'go.sum', 'Dockerfile', '.dockerignore', 'LICENSE', 'NOTICE', 'UPSTREAM.md')
    foreach ($name in $serverFiles) {
        $source = Join-Path $workspace "server\$name"
        if (-not (Test-Path -LiteralPath $source -PathType Leaf)) {
            throw "Required server build file not found: $source"
        }
        Copy-Item -LiteralPath $source -Destination $serverTarget
    }
    foreach ($name in @('cmd', 'db', 'docs', 'internal', 'pkg')) {
        $source = Join-Path $workspace "server\$name"
        if (-not (Test-Path -LiteralPath $source -PathType Container)) {
            throw "Required server build directory not found: $source"
        }
        Copy-Item -LiteralPath $source -Destination $serverTarget -Recurse
    }
    $webTarget = Join-Path $serverTarget 'web'
    New-Item -ItemType Directory -Path $webTarget -Force | Out-Null
    foreach ($name in @('package.json', 'bun.lock', 'index.html', 'vite.config.ts', 'tsconfig.json', 'tsconfig.app.json', 'tsconfig.node.json', 'biome.json')) {
        $source = Join-Path $workspace "server\web\$name"
        if (-not (Test-Path -LiteralPath $source -PathType Leaf)) {
            throw "Required web build file not found: $source"
        }
        Copy-Item -LiteralPath $source -Destination $webTarget
    }
    foreach ($name in @('public', 'src')) {
        $source = Join-Path $workspace "server\web\$name"
        if (-not (Test-Path -LiteralPath $source -PathType Container)) {
            throw "Required web build directory not found: $source"
        }
        Copy-Item -LiteralPath $source -Destination $webTarget -Recurse
    }

    $lines = foreach ($file in Get-ChildItem -LiteralPath $destinationPath -File -Recurse) {
        $relative = [IO.Path]::GetRelativePath($destinationPath, $file.FullName).Replace('\', '/')
        $hash = (Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
        "$hash  $relative"
    }
    Set-Content -LiteralPath (Join-Path $destinationPath 'migration-checksums.sha256') -Value $lines -Encoding utf8NoBOM
    Write-Output "MIGRATION_BUNDLE_READY $destinationPath"
}
