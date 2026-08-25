$ErrorActionPreference = 'Stop'
$repoRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$sourceFixture = Join-Path $repoRoot 'deploy\lib.sh'
$destination = Join-Path ([IO.Path]::GetTempPath()) ('licensehub-bundle-' + [Guid]::NewGuid().ToString('N'))
$original = [IO.File]::ReadAllBytes($sourceFixture)
$utf8NoBom = [Text.UTF8Encoding]::new($false)

try {
    # Prove the bundler repairs a Windows-style source checkout instead of
    # merely passing when CI happened to check the repository out with LF.
    $crlf = ([Text.Encoding]::UTF8.GetString($original)).Replace("`r`n", "`n").Replace("`n", "`r`n")
    [IO.File]::WriteAllText($sourceFixture, $crlf, $utf8NoBom)

    & (Join-Path $repoRoot 'scripts\Migrate-LicenseHubVps.ps1') -Destination $destination -SkipBackup | Out-Host
    & (Join-Path $repoRoot 'scripts\Verify-MigrationBundle.ps1') -Bundle $destination | Out-Host

    $linuxFiles = @(Get-ChildItem -LiteralPath $destination -Recurse -File | Where-Object {
        $_.Name -in @('Dockerfile', '.dockerignore') -or $_.Extension -in @('.sh', '.sql', '.yml', '.yaml')
    })
    $bad = @($linuxFiles | Where-Object { [IO.File]::ReadAllBytes($_.FullName) -contains 13 })
    if ($bad.Count) {
        throw "CR bytes remain in Linux bundle inputs: $($bad.FullName -join ', ')"
    }
    if (-not $IsWindows) {
        & bash -n @($linuxFiles | Where-Object Extension -eq '.sh' | ForEach-Object FullName)
        if ($LASTEXITCODE -ne 0) { throw 'A staged Linux shell script failed bash -n.' }
    }
    Write-Output "PASS migration bundle is verified and LF-only ($($linuxFiles.Count) Linux inputs)"
}
finally {
    [IO.File]::WriteAllBytes($sourceFixture, $original)
    if (Test-Path -LiteralPath $destination) {
        Remove-Item -LiteralPath $destination -Recurse -Force
    }
}
