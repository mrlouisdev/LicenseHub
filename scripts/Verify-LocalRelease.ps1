[CmdletBinding()]
param(
    [switch]$KeepTestContainers,
    [switch]$SkipPortable
)

$ErrorActionPreference = 'Stop'
$workspace = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$suffix = [guid]::NewGuid().ToString('N').Substring(0, 12)
$postgresName = "licensehub-verify-pg-$suffix"
$redisName = "licensehub-verify-redis-$suffix"
$portableRoot = Join-Path $workspace "artifacts\verify-$suffix"

function Get-FreePort {
    $listener = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback, 0)
    $listener.Start()
    try { return ([Net.IPEndPoint]$listener.LocalEndpoint).Port } finally { $listener.Stop() }
}

function Invoke-Checked([scriptblock]$Action, [string]$Name) {
    & $Action
    if ($LASTEXITCODE -ne 0) { throw "$Name failed with exit code $LASTEXITCODE" }
    Write-Output "PASS $Name"
}

function Get-LockValue([string]$Name) {
    foreach ($line in Get-Content -LiteralPath (Join-Path $workspace 'deploy\images.lock')) {
        if ($line -match ('^' + [regex]::Escape($Name) + '=(.+)$')) { return $Matches[1] }
    }
    throw "Image lock is missing $Name"
}

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) { throw 'docker is required' }
docker info *> $null
if ($LASTEXITCODE -ne 0) { throw 'Docker daemon is unavailable; start Docker Desktop and rerun this verifier' }
$postgresPort = Get-FreePort
$redisPort = Get-FreePort
$postgresImage = Get-LockValue 'POSTGRES_IMAGE'
$redisImage = Get-LockValue 'REDIS_IMAGE'

try {
    Invoke-Checked { docker run -d --rm --name $postgresName -e POSTGRES_USER=licensehub -e POSTGRES_PASSWORD=local_fixture_password -e POSTGRES_DB=licensehub_test -p "127.0.0.1:${postgresPort}:5432" $postgresImage | Out-Null } 'start PostgreSQL fixture'
    Invoke-Checked { docker run -d --rm --name $redisName -p "127.0.0.1:${redisPort}:6379" $redisImage | Out-Null } 'start Redis fixture'

    $ready = $false
    for ($attempt = 0; $attempt -lt 60; $attempt++) {
        docker exec $postgresName pg_isready -U licensehub -d licensehub_test *> $null
        $pgReady = $LASTEXITCODE -eq 0
        docker exec $redisName redis-cli ping *> $null
        $redisReady = $LASTEXITCODE -eq 0
        if ($pgReady -and $redisReady) { $ready = $true; break }
        Start-Sleep -Seconds 1
    }
    if (-not $ready) { throw 'Database fixtures did not become ready' }

    $env:TEST_DATABASE_URL = "postgres://licensehub:local_fixture_password@127.0.0.1:$postgresPort/licensehub_test?sslmode=disable"
    $env:TEST_REDIS_URL = "redis://127.0.0.1:$redisPort/0"
    Invoke-Checked { Push-Location (Join-Path $workspace 'server'); try { go test -count=1 -p 1 -timeout 12m ./... } finally { Pop-Location } } 'Go full suite'
    Invoke-Checked { Push-Location (Join-Path $workspace 'server'); try { go vet ./... } finally { Pop-Location } } 'Go vet'
    Invoke-Checked { Push-Location (Join-Path $workspace 'core'); try { cargo test --workspace } finally { Pop-Location } } 'Rust workspace'
    Invoke-Checked { dotnet build (Join-Path $workspace 'cli\licctl\licctl.csproj') -c Release } 'licctl Release build'
    Invoke-Checked { & (Join-Path $workspace 'tests\integration\Run-LicctlIntegration.ps1') } 'four-stack integration'
    Invoke-Checked {
        Push-Location (Join-Path $workspace 'server\web')
        try { bun run lint; if ($LASTEXITCODE) { return }; bun run typecheck; if ($LASTEXITCODE) { return }; bun run build } finally { Pop-Location }
    } 'server web lint/typecheck/build'
    Invoke-Checked { bash -n deploy/*.sh tests/deployment/*.sh } 'shell syntax'
    Invoke-Checked { bash tests/deployment/Test-RestoreRollback.sh } 'restore rollback failure path'
    Invoke-Checked { docker compose --env-file (Join-Path $workspace 'deploy\.env.example') -f (Join-Path $workspace 'deploy\docker-compose.yml') config --quiet } 'standalone Compose config'
    $env:EXTERNAL_EDGE_NETWORK = 'licensehub-verify-edge'
    Invoke-Checked { docker compose --env-file (Join-Path $workspace 'deploy\.env.example') -f (Join-Path $workspace 'deploy\docker-compose.integrated.yml') config --quiet } 'integrated Compose config'
    Invoke-Checked { & (Join-Path $workspace 'scripts\Verify-Release.ps1') } 'release evidence manifest'

    if (-not $SkipPortable) {
        & (Join-Path $workspace 'scripts\Build-LicctlPortable.ps1') -OutputRoot $portableRoot | Out-Host
        & (Join-Path $workspace 'scripts\Verify-LicctlPortable.ps1') -Archive (Join-Path $portableRoot 'licctl-portable-win-x64.zip') | Out-Host
        Write-Output 'PASS portable licctl outside-source verification'
    }
    Write-Output 'LOCAL_RELEASE_VERIFICATION_OK'
} finally {
    Remove-Item Env:TEST_DATABASE_URL -ErrorAction SilentlyContinue
    Remove-Item Env:TEST_REDIS_URL -ErrorAction SilentlyContinue
    Remove-Item Env:EXTERNAL_EDGE_NETWORK -ErrorAction SilentlyContinue
    if (-not $KeepTestContainers) {
        docker rm -f $postgresName $redisName *> $null
    }
    if (Test-Path -LiteralPath $portableRoot) { Remove-Item -LiteralPath $portableRoot -Recurse -Force }
}
