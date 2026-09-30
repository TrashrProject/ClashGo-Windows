param(
    [Parameter(Mandatory=$true)][string]$Version,
    [string]$PfxBase64 = $env:WINDOWS_SIGNING_PFX_BASE64,
    [string]$PfxPassword = $env:WINDOWS_SIGNING_PFX_PASSWORD,
    [string]$TimestampURL = "http://timestamp.digicert.com"
)

$ErrorActionPreference = "Stop"
$repoRoot = Split-Path -Parent $PSScriptRoot
$bundle = Join-Path $repoRoot "dist\ClashGO-Windows"
$exe = Join-Path $bundle "ClashGO.exe"
$zip = Join-Path $repoRoot ("dist\ClashGO-v{0}-windows.zip" -f $Version)
$setup = Join-Path $repoRoot ("dist\ClashGO-v{0}-windows-setup.exe" -f $Version)

if ([string]::IsNullOrWhiteSpace($PfxBase64) -or [string]::IsNullOrWhiteSpace($PfxPassword)) {
    Write-Host "Windows signing secret not configured; leaving artifacts unsigned."
    exit 0
}
if (-not (Test-Path -LiteralPath $exe)) {
    throw "Portable ClashGO.exe not found: $exe"
}

$windowsKits = Join-Path ([Environment]::GetFolderPath("ProgramFilesX86")) "Windows Kits\10\bin"
$signtool = Get-ChildItem $windowsKits -Recurse -Filter signtool.exe -ErrorAction SilentlyContinue |
    Where-Object { $_.FullName -match '\\x64\\signtool\.exe$' } |
    Sort-Object FullName -Descending |
    Select-Object -First 1
if (-not $signtool) {
    throw "signtool.exe was not found in the Windows SDK"
}

$pfx = Join-Path $env:RUNNER_TEMP ("clashgo-signing-" + [guid]::NewGuid().ToString("N") + ".pfx")
try {
    [System.IO.File]::WriteAllBytes($pfx, [Convert]::FromBase64String($PfxBase64))

    & $signtool.FullName sign /fd SHA256 /td SHA256 /tr $TimestampURL /f $pfx /p $PfxPassword $exe
    if ($LASTEXITCODE -ne 0) { throw "Authenticode signing failed for ClashGO.exe" }

    & $signtool.FullName verify /pa /all $exe
    if ($LASTEXITCODE -ne 0) { throw "Authenticode verification failed for ClashGO.exe" }

    if (Test-Path -LiteralPath $zip) { Remove-Item -LiteralPath $zip -Force }
    Compress-Archive -Path $bundle -DestinationPath $zip -CompressionLevel Optimal

    if (Test-Path -LiteralPath $setup) {
        & $signtool.FullName sign /fd SHA256 /td SHA256 /tr $TimestampURL /f $pfx /p $PfxPassword $setup
        if ($LASTEXITCODE -ne 0) { throw "Authenticode signing failed for installer" }
        & $signtool.FullName verify /pa /all $setup
        if ($LASTEXITCODE -ne 0) { throw "Authenticode verification failed for installer" }
    }

    Write-Host "Authenticode signing complete."
}
finally {
    Remove-Item -LiteralPath $pfx -Force -ErrorAction SilentlyContinue
}
