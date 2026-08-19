[CmdletBinding(SupportsShouldProcess = $true)]
param(
    [string]$ComposeFile = (Join-Path $PSScriptRoot '..\deploy\docker-compose.yml'),
    [string]$OutputRoot = (Join-Path $PSScriptRoot '..\backups'),
    [switch]$DryRun
)

$ErrorActionPreference = 'Stop'
$stamp = (Get-Date).ToUniversalTime().ToString('yyyyMMddTHHmmssZ')
$compose = [IO.Path]::GetFullPath($ComposeFile)
$output = [IO.Path]::GetFullPath((Join-Path $OutputRoot $stamp))

if (-not (Test-Path -LiteralPath $compose -PathType Leaf)) {
    throw "Compose file not found: $compose"
}

$plan = [ordered]@{
    operation = 'backup'
    compose_file = $compose
    output_directory = $output
    contents = @('database.dump', 'backup-manifest.json', 'checksums.sha256')
}
if ($DryRun) {
    $plan | ConvertTo-Json
    return
}
if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    throw 'docker is required'
}

if ($PSCmdlet.ShouldProcess($output, 'Create PostgreSQL backup')) {
    New-Item -ItemType Directory -Path $output -Force | Out-Null
    $containerDump = '/tmp/licensehub-backup.dump'
    $dump = Join-Path $output 'database.dump'

    & docker compose -f $compose exec -T postgres sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc -f /tmp/licensehub-backup.dump'
    if ($LASTEXITCODE -ne 0) { throw 'pg_dump failed' }
    & docker compose -f $compose cp "postgres:$containerDump" $dump
    if ($LASTEXITCODE -ne 0) { throw 'copying the dump failed' }
    & docker compose -f $compose exec -T postgres rm -f $containerDump
    if (-not (Test-Path -LiteralPath $dump) -or (Get-Item $dump).Length -eq 0) {
        throw 'Backup dump is missing or empty'
    }

    $manifestPath = Join-Path $output 'backup-manifest.json'
    [ordered]@{
        format_version = 1
        created_at_utc = (Get-Date).ToUniversalTime().ToString('o')
        server_version = $env:LICENSEHUB_VERSION
        files = @('database.dump')
    } | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath $manifestPath -Encoding utf8NoBOM

    $checksumLines = foreach ($file in @($dump, $manifestPath)) {
        $hash = (Get-FileHash -LiteralPath $file -Algorithm SHA256).Hash.ToLowerInvariant()
        "$hash  $([IO.Path]::GetFileName($file))"
    }
    Set-Content -LiteralPath (Join-Path $output 'checksums.sha256') -Value $checksumLines -Encoding utf8NoBOM
    Write-Output "BACKUP_READY $output"
}

