[CmdletBinding(SupportsShouldProcess = $true)]
param(
    [string]$ComposeFile = (Join-Path $PSScriptRoot '..\deploy\docker-compose.yml'),
    [string]$EnvironmentFile = (Join-Path $PSScriptRoot '..\deploy\.env'),
    [string]$OutputRoot = (Join-Path $PSScriptRoot '..\backups'),
    [switch]$DryRun
)

$ErrorActionPreference = 'Stop'
$stamp = (Get-Date).ToUniversalTime().ToString('yyyyMMddTHHmmssZ')
$compose = [IO.Path]::GetFullPath($ComposeFile)
$environment = [IO.Path]::GetFullPath($EnvironmentFile)
$output = [IO.Path]::GetFullPath((Join-Path $OutputRoot $stamp))

if (-not (Test-Path -LiteralPath $compose -PathType Leaf)) {
    throw "Compose file not found: $compose"
}
if (-not (Test-Path -LiteralPath $environment -PathType Leaf)) {
    throw "Environment file not found: $environment"
}

function Get-EnvironmentValue([string]$Name) {
    $prefix = [regex]::Escape($Name)
    foreach ($line in Get-Content -LiteralPath $environment) {
        if ($line -match "^$prefix=(.*)$") { return $Matches[1] }
    }
    return $null
}

$ageRecipient = Get-EnvironmentValue 'BACKUP_AGE_RECIPIENT'
if ($ageRecipient -notmatch '^age1[023456789acdefghjklmnpqrstuvwxyz]{58}$') {
    throw 'BACKUP_AGE_RECIPIENT must be a valid age X25519 public recipient'
}

$plan = [ordered]@{
    operation = 'backup'
    compose_file = $compose
    environment_file = $environment
    output_directory = $output
    contents = @('database.dump', 'recovery.env.age', 'image-lock.txt', 'backup-manifest.json', 'checksums.sha256')
}
if ($DryRun) {
    $plan | ConvertTo-Json
    return
}
if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    throw 'docker is required'
}
if (-not (Get-Command age -ErrorAction SilentlyContinue)) {
    throw 'age is required for encrypted recovery backups'
}

if ($PSCmdlet.ShouldProcess($output, 'Create PostgreSQL backup')) {
    New-Item -ItemType Directory -Path $output -Force | Out-Null
    $containerDump = '/tmp/licensehub-backup-' + [guid]::NewGuid().ToString('N') + '.dump'
    $dump = Join-Path $output 'database.dump'

    & docker compose --env-file $environment -f $compose up -d postgres
    $pgDumpCommand = 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc -f ' + $containerDump
    & docker compose --env-file $environment -f $compose exec -T postgres sh -c $pgDumpCommand
    if ($LASTEXITCODE -ne 0) { throw 'pg_dump failed' }
    & docker compose --env-file $environment -f $compose cp "postgres:$containerDump" $dump
    if ($LASTEXITCODE -ne 0) { throw 'copying the dump failed' }
    & docker compose --env-file $environment -f $compose exec -T postgres rm -f $containerDump
    if (-not (Test-Path -LiteralPath $dump) -or (Get-Item $dump).Length -eq 0) {
        throw 'Backup dump is missing or empty'
    }

    $recovery = Join-Path $output 'recovery.env.age'
    & age --encrypt --recipient $ageRecipient --output $recovery $environment
    if ($LASTEXITCODE -ne 0 -or -not (Test-Path -LiteralPath $recovery) -or (Get-Item $recovery).Length -eq 0) {
        throw 'encrypted recovery bundle is missing or empty'
    }

    $imageLock = Join-Path $output 'image-lock.txt'
    & docker compose --env-file $environment -f $compose config --images | Sort-Object -Unique | Set-Content -LiteralPath $imageLock -Encoding utf8NoBOM
    if (-not (Test-Path -LiteralPath $imageLock) -or (Get-Item $imageLock).Length -eq 0) {
        throw 'image lock metadata is missing or empty'
    }

    $manifestPath = Join-Path $output 'backup-manifest.json'
    [ordered]@{
        format_version = 2
        created_at_utc = (Get-Date).ToUniversalTime().ToString('o')
        server_version = $env:LICENSEHUB_VERSION
        encryption = 'age-x25519'
        recovery_bundle = 'recovery.env.age'
        files = @('database.dump', 'recovery.env.age', 'image-lock.txt')
    } | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath $manifestPath -Encoding utf8NoBOM

    $checksumLines = foreach ($file in @($dump, $recovery, $imageLock, $manifestPath)) {
        $hash = (Get-FileHash -LiteralPath $file -Algorithm SHA256).Hash.ToLowerInvariant()
        "$hash  $([IO.Path]::GetFileName($file))"
    }
    Set-Content -LiteralPath (Join-Path $output 'checksums.sha256') -Value $checksumLines -Encoding utf8NoBOM
    Write-Output "BACKUP_READY $output"
}
