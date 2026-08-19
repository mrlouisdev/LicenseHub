[CmdletBinding(SupportsShouldProcess = $true, ConfirmImpact = 'High')]
param(
    [Parameter(Mandatory)] [string]$BackupDirectory,
    [string]$ComposeFile = (Join-Path $PSScriptRoot '..\deploy\docker-compose.yml'),
    [string]$SafetyBackupRoot = (Join-Path $PSScriptRoot '..\backups\pre-restore-safety'),
    [switch]$Force,
    [switch]$DryRun
)

$ErrorActionPreference = 'Stop'
$backup = [IO.Path]::GetFullPath($BackupDirectory)
$backupPrefix = $backup.TrimEnd([IO.Path]::DirectorySeparatorChar) + [IO.Path]::DirectorySeparatorChar
$compose = [IO.Path]::GetFullPath($ComposeFile)
$safetyRoot = [IO.Path]::GetFullPath($SafetyBackupRoot)
$dump = Join-Path $backup 'database.dump'
$checksums = Join-Path $backup 'checksums.sha256'

foreach ($required in @($compose, $dump, $checksums)) {
    if (-not (Test-Path -LiteralPath $required -PathType Leaf)) {
        throw "Required file not found: $required"
    }
}

$failures = @()
foreach ($line in Get-Content -LiteralPath $checksums) {
    if ($line -notmatch '^([a-fA-F0-9]{64})  (.+)$') {
        throw "Invalid checksum line: $line"
    }
    $path = [IO.Path]::GetFullPath((Join-Path $backup $Matches[2]))
    if (-not $path.StartsWith($backupPrefix, [StringComparison]::OrdinalIgnoreCase)) {
        throw "Checksum entry escapes backup directory: $($Matches[2])"
    }
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        $failures += "missing $($Matches[2])"
        continue
    }
    $actual = (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash
    if ($actual -ne $Matches[1]) { $failures += "mismatch $($Matches[2])" }
}
if ($failures.Count) { throw "Checksum verification failed: $($failures -join ', ')" }

$plan = [ordered]@{
    operation = 'restore'
    backup_directory = $backup
    compose_file = $compose
    destructive = $true
    checksums = 'verified'
    pre_restore_safety_backup = $safetyRoot
    atomic_restore = 'pg_restore --single-transaction --exit-on-error'
    failure_behavior = 'restore safety dump and restart application services'
}
if ($DryRun) { $plan | ConvertTo-Json; return }
if (-not $Force) {
    throw 'Restore replaces database contents. Re-run with -Force after reviewing -DryRun output.'
}
if (-not (Get-Command docker -ErrorAction SilentlyContinue)) { throw 'docker is required' }

$containerDump = '/tmp/licensehub-restore.dump'
$containerSafetyDump = '/tmp/licensehub-pre-restore.dump'
if ($PSCmdlet.ShouldProcess('LicenseHub PostgreSQL database', "Restore $dump")) {
    # Validate that pg_restore can read the archive before stopping application
    # traffic or touching the destination database.
    & docker compose -f $compose up -d postgres
    if ($LASTEXITCODE -ne 0) { throw 'Failed to start postgres' }
    & docker compose -f $compose cp $dump "postgres:$containerDump"
    if ($LASTEXITCODE -ne 0) { throw 'Failed to copy dump into postgres container' }
    & docker compose -f $compose exec -T postgres pg_restore --list $containerDump | Out-Null
    if ($LASTEXITCODE -ne 0) {
        & docker compose -f $compose exec -T postgres rm -f $containerDump
        throw 'Input archive failed pg_restore preflight; database was not modified'
    }

    $stamp = (Get-Date).ToUniversalTime().ToString('yyyyMMddTHHmmssZ')
    $safetyDirectory = Join-Path $safetyRoot $stamp
    $safetyDump = Join-Path $safetyDirectory 'database.dump'
    $serverStopped = $false
    $safetyReady = $false
    try {
        & docker compose -f $compose stop server
        if ($LASTEXITCODE -ne 0) { throw 'Failed to stop server before safety backup' }
        $serverStopped = $true

        New-Item -ItemType Directory -Path $safetyDirectory -Force | Out-Null
        & docker compose -f $compose exec -T postgres sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc -f /tmp/licensehub-pre-restore.dump'
        if ($LASTEXITCODE -ne 0) { throw 'Pre-restore safety pg_dump failed' }
        & docker compose -f $compose cp "postgres:$containerSafetyDump" $safetyDump
        if ($LASTEXITCODE -ne 0 -or -not (Test-Path -LiteralPath $safetyDump) -or (Get-Item $safetyDump).Length -eq 0) {
            throw 'Pre-restore safety dump is missing or empty'
        }
        $safetyHash = (Get-FileHash -LiteralPath $safetyDump -Algorithm SHA256).Hash.ToLowerInvariant()
        Set-Content -LiteralPath (Join-Path $safetyDirectory 'checksums.sha256') -Value "$safetyHash  database.dump" -Encoding utf8NoBOM
        $safetyReady = $true

        # A single transaction prevents a failed archive from leaving a
        # half-restored schema. The safety dump remains a second rollback layer.
        & docker compose -f $compose exec -T postgres sh -c 'pg_restore -U "$POSTGRES_USER" -d "$POSTGRES_DB" --clean --if-exists --no-owner --no-privileges --single-transaction --exit-on-error /tmp/licensehub-restore.dump'
        if ($LASTEXITCODE -ne 0) { throw 'Atomic pg_restore failed' }

        & docker compose -f $compose up -d server caddy
        if ($LASTEXITCODE -ne 0) { throw 'Database restored but application restart failed' }
        $serverStopped = $false
        Write-Output "RESTORE_COMPLETE $backup SAFETY_BACKUP $safetyDirectory"
    }
    catch {
        $restoreError = $_
        $rollbackError = $null
        if ($safetyReady) {
            & docker compose -f $compose exec -T postgres sh -c 'pg_restore -U "$POSTGRES_USER" -d "$POSTGRES_DB" --clean --if-exists --no-owner --no-privileges --single-transaction --exit-on-error /tmp/licensehub-pre-restore.dump'
            if ($LASTEXITCODE -ne 0) { $rollbackError = 'safety rollback failed' }
        }
        & docker compose -f $compose up -d server caddy
        if ($LASTEXITCODE -ne 0) {
            $rollbackError = if ($rollbackError) { "$rollbackError; application restart failed" } else { 'application restart failed' }
        } else {
            $serverStopped = $false
        }
        if ($rollbackError) {
            throw "$($restoreError.Exception.Message); $rollbackError; safety dump: $safetyDump"
        }
        throw "$($restoreError.Exception.Message); previous database restored and services restarted"
    }
    finally {
        & docker compose -f $compose exec -T postgres rm -f $containerDump $containerSafetyDump 2>$null
        if ($serverStopped) {
            & docker compose -f $compose up -d server caddy 2>$null
        }
    }
}
