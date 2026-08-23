[CmdletBinding()]
param([switch]$KeepTemporary)
$ErrorActionPreference = 'Stop'
$root = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$cli = Join-Path $root 'cli/licctl/bin/Debug/net8.0/licctl.dll'
dotnet build (Join-Path $root 'cli/licctl/licctl.csproj') --nologo | Out-Host
if ($LASTEXITCODE) { throw "licctl build failed" }
$temporary = Join-Path ([IO.Path]::GetTempPath()) ('licensehub-integration-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $temporary | Out-Null
$mock = $null
try {
  $probe = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback, 0)
  $probe.Start(); $port = ([Net.IPEndPoint]$probe.LocalEndpoint).Port; $probe.Stop()
  $kit = Join-Path $temporary 'kit'
  $key = [Convert]::ToBase64String([byte[]](0..31))
  & dotnet $cli init --product fixture --server "http://127.0.0.1:$port" --public-key "fixture=$key" --out $kit
  if ($LASTEXITCODE) { throw 'init failed' }
  & dotnet $cli doctor --kit $kit --json
  if ($LASTEXITCODE) { throw 'kit doctor failed' }
  $profile = Get-Content (Join-Path $kit 'product.profile.json') -Raw | ConvertFrom-Json
  if ($profile.PSObject.Properties.Name -contains 'stack') { throw 'product profile must remain stack-agnostic' }
  foreach ($payload in 'bindings/dotnet','bindings/node','bindings/python','bindings/cpp','core/include/license_core.h','core/target/release/license_core.dll') {
    if (-not (Test-Path (Join-Path $kit $payload))) { throw "portable kit payload is missing $payload" }
  }
  Add-Type -AssemblyName System.IO.Compression.FileSystem
  $kitZip = Join-Path $temporary 'licensehub-kit.zip'
  [IO.Compression.ZipFile]::CreateFromDirectory($kit, $kitZip, [IO.Compression.CompressionLevel]::NoCompression, $false)

  $mock = Start-Job -ArgumentList $port,$key -ScriptBlock {
    param($port,$key)
    $listener = [Net.HttpListener]::new()
    $listener.Prefixes.Add("http://127.0.0.1:$port/")
    $listener.Start()
    try {
      while ($listener.IsListening) {
        $context = $listener.GetContext()
        $path = $context.Request.Url.AbsolutePath
        $shutdown = $path -eq '/__shutdown'
        $body = switch ($path) {
          '/v1/client/public-keys' { '{"keys":{"fixture":"' + $key + '"}}' }
          '/v1/client/activate' { '{"lease":"fixture-lease","entitlements":["fixture.pro"]}' }
          '/v1/client/refresh' { '{"lease":"fixture-refreshed","entitlements":["fixture.pro"]}' }
          '/v1/client/deactivate' { '{"ok":true}' }
          '/__shutdown' { '{"ok":true}' }
          default { $context.Response.StatusCode = 404; '{"error":"not found"}' }
        }
        $bytes = [Text.Encoding]::UTF8.GetBytes($body)
        $context.Response.ContentType = 'application/json'
        $context.Response.ContentLength64 = $bytes.Length
        $context.Response.OutputStream.Write($bytes,0,$bytes.Length)
        $context.Response.Close()
        if ($shutdown) { break }
      }
    } finally { $listener.Close() }
  }
  Start-Sleep -Milliseconds 500
  foreach ($stack in 'dotnet','electron','python','cpp') {
    $project = Join-Path $temporary $stack
    Copy-Item (Join-Path $PSScriptRoot "fixtures/$stack") $project -Recurse
    $before = Get-ChildItem $project -Recurse -File | Sort-Object FullName | ForEach-Object { "$($_.FullName.Substring($project.Length)):$((Get-FileHash $_.FullName -Algorithm SHA256).Hash)" }
    & dotnet $cli add --project $project --kit $kitZip
    if ($LASTEXITCODE) { throw "$stack add failed" }
    $first = (Get-FileHash (Join-Path $project '.licensehub/install.json') -Algorithm SHA256).Hash
    $noop = (& dotnet $cli add --project $project --kit $kitZip | Out-String)
    if ($LASTEXITCODE) { throw "$stack second add failed" }
    if ($noop -notmatch '^NOOP ') { throw "$stack second add did not report NOOP" }
    $second = (Get-FileHash (Join-Path $project '.licensehub/install.json') -Algorithm SHA256).Hash
    if ($first -ne $second) { throw "$stack second add was not a byte-for-byte NOOP" }
    & dotnet $cli doctor --project $project --json
    if ($LASTEXITCODE) { throw "$stack doctor failed" }
    $manifest = Get-Content (Join-Path $project '.licensehub/install.json') -Raw | ConvertFrom-Json
    $modified = @($manifest.files | Where-Object kind -eq 'modified')
    if ($manifest.stack -ne $stack -or -not $manifest.files -or
        @($manifest.files | Where-Object { -not $_.path -or -not $_.kind -or -not $_.installed_sha256 }).Count -or
        @($modified | Where-Object { -not $_.original_sha256 -or -not $_.backup }).Count) {
      throw "$stack install manifest lacks detected stack/files/rollback"
    }
    & dotnet $cli verify --project $project
    if ($LASTEXITCODE) { throw "$stack local verify failed" }
    'fixture-activation' | & dotnet $cli verify --project $project --live --activation-stdin --entitlement fixture.pro --json
    if ($LASTEXITCODE) { throw "$stack live lifecycle verify failed" }
    & dotnet $cli remove --project $project
    if ($LASTEXITCODE) { throw "$stack remove failed" }
    $after = Get-ChildItem $project -Recurse -File | Where-Object { $_.FullName -notmatch '[\\/](bin|obj)[\\/]' } | Sort-Object FullName | ForEach-Object { "$($_.FullName.Substring($project.Length)):$((Get-FileHash $_.FullName -Algorithm SHA256).Hash)" }
    if (Compare-Object $before $after) { throw "$stack remove did not restore a clean file diff" }
  }
  'PASS all four licctl integration fixtures'
} finally {
  if ($null -ne $mock) {
    try { Invoke-WebRequest "http://127.0.0.1:$port/__shutdown" -TimeoutSec 2 | Out-Null } catch {}
    Wait-Job $mock -Timeout 5 -ErrorAction SilentlyContinue | Out-Null
    Remove-Job $mock -Force -ErrorAction SilentlyContinue
  }
  if (-not $KeepTemporary -and (Test-Path $temporary)) { Remove-Item -LiteralPath $temporary -Recurse -Force }
}
