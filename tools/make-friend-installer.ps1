param(
    [string]$Version = "0.6.5-windows-beta",
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
    $installed = $false

    # Prefer winget because it is available on normal Windows 10/11 installs
    # and avoids requiring Chocolatey just to build the installer.
    $winget = Get-Command winget.exe -ErrorAction SilentlyContinue
    if ($winget) {
        Write-Host "NSIS absent - installation automatique via winget..."
        & $winget.Source install --id NSIS.NSIS -e --silent --accept-package-agreements --accept-source-agreements
        if ($LASTEXITCODE -eq 0) {
            $installed = $true
        } else {
            Write-Host "winget n'a pas pu installer NSIS; tentative via Chocolatey..."
        }
    }

    if (-not $installed) {
        $choco = Get-Command choco.exe -ErrorAction SilentlyContinue
        if ($choco) {
            Write-Host "Installation automatique de NSIS via Chocolatey..."
            & $choco.Source install nsis -y --no-progress
            if ($LASTEXITCODE -eq 0) {
                $installed = $true
            }
        }
    }

    # Refresh the common NSIS install locations in the current PowerShell.
    $nsisCandidates = @()
    if ($env:ProgramFiles) { $nsisCandidates += (Join-Path $env:ProgramFiles "NSIS") }
    $pf86 = [Environment]::GetEnvironmentVariable("ProgramFiles(x86)")
    if ($pf86) { $nsisCandidates += (Join-Path $pf86 "NSIS") }
    foreach ($candidate in $nsisCandidates) {
        if (Test-Path (Join-Path $candidate "makensis.exe")) {
            $env:Path = "$candidate;$env:Path"
            break
        }
    }

    $makensis = Get-Command makensis.exe -ErrorAction SilentlyContinue
    if (-not $makensis) {
        throw "Installation automatique de NSIS impossible. Installe NSIS puis relance ce script."
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
