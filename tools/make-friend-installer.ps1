param(
    [string]$Version = "0.6.0-windows-beta",
    [switch]$SkipTests
)

$ErrorActionPreference = "Stop"
$repoRoot = Split-Path -Parent $PSScriptRoot
Set-Location $repoRoot

Write-Host ""
Write-Host "=== ClashGO - package ami ==="
Write-Host "Version: $Version"

# Friend builds should never inherit the developer shell's account-service
# setting. Clear it for the child build and pass an explicit empty value.
$previousAccountServiceURL = $env:CLASHGO_ACCOUNT_API_URL
try {
    Remove-Item Env:CLASHGO_ACCOUNT_API_URL -ErrorAction SilentlyContinue
    & (Join-Path $repoRoot "tools\build-windows.ps1") -Version $Version -AccountServiceURL "" -SkipTests:$SkipTests
    if ($LASTEXITCODE -ne 0) { throw "ClashGO Windows build failed." }
}
finally {
    if ($null -ne $previousAccountServiceURL -and $previousAccountServiceURL -ne "") {
        $env:CLASHGO_ACCOUNT_API_URL = $previousAccountServiceURL
    } else {
        Remove-Item Env:CLASHGO_ACCOUNT_API_URL -ErrorAction SilentlyContinue
    }
}

$makensis = Get-Command makensis.exe -ErrorAction SilentlyContinue
if (-not $makensis) {
    $choco = Get-Command choco.exe -ErrorAction SilentlyContinue
    if ($choco) {
        Write-Host "NSIS absent - installation automatique via Chocolatey..."
        & $choco.Source install nsis -y --no-progress
        if ($LASTEXITCODE -ne 0) { throw "NSIS installation failed." }
        $env:Path += ";$env:ProgramFiles\NSIS;$env:ProgramFiles(x86)\NSIS"
    } else {
        throw "NSIS est requis pour produire Setup.exe. Installe NSIS puis relance ce script."
    }
}

& (Join-Path $repoRoot "tools\build-installer.ps1") -Version $Version
if ($LASTEXITCODE -ne 0) { throw "Installer build failed." }

$setup = Join-Path $repoRoot ("dist\ClashGO-v{0}-windows-setup.exe" -f $Version)
$zip = Join-Path $repoRoot ("dist\ClashGO-v{0}-windows.zip" -f $Version)
if (-not (Test-Path $setup)) { throw "Setup not found: $setup" }

$sha = (Get-FileHash -LiteralPath $setup -Algorithm SHA256).Hash.ToLowerInvariant()
$sumPath = Join-Path $repoRoot ("dist\ClashGO-v{0}-windows-setup.sha256.txt" -f $Version)
"$sha  $([IO.Path]::GetFileName($setup))" | Set-Content -LiteralPath $sumPath -Encoding ASCII

Write-Host ""
Write-Host "PACKAGE AMI PRET"
Write-Host "Installer : $setup"
if (Test-Path $zip) { Write-Host "Portable  : $zip" }
Write-Host "SHA256    : $sha"
Write-Host ""
Write-Host "Tu peux envoyer uniquement le fichier Setup.exe a ton ami."
