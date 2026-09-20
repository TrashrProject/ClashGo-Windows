param(
    [string]$Version = "0.6.0-windows-beta",
    [string]$BundlePath = "",
    [string]$OutputPath = ""
)

$ErrorActionPreference = "Stop"
$repoRoot = Split-Path -Parent $PSScriptRoot

if (-not $BundlePath) { $BundlePath = Join-Path $repoRoot "dist\ClashGO-Windows" }
if (-not (Test-Path (Join-Path $BundlePath "ClashGO.exe"))) {
    throw "Portable bundle is missing at $BundlePath. Run tools\build-windows.ps1 first."
}

$makensis = Get-Command makensis.exe -ErrorAction SilentlyContinue
$makensisPath = if ($makensis) { $makensis.Source } else { $null }
if (-not $makensisPath) {
    $candidates = @()

    $programFilesX86 = [Environment]::GetEnvironmentVariable("ProgramFiles(x86)")
    if ($programFilesX86) {
        $candidates += (Join-Path $programFilesX86 "NSIS\makensis.exe")
    }

    $programFiles = [Environment]::GetEnvironmentVariable("ProgramFiles")
    if ($programFiles) {
        $candidates += (Join-Path $programFiles "NSIS\makensis.exe")
    }

    $choco = [Environment]::GetEnvironmentVariable("ChocolateyInstall")
    if ($choco) {
        $candidates += (Join-Path $choco "bin\makensis.exe")
    }

    foreach ($candidate in $candidates) {
        if ($candidate -and (Test-Path $candidate)) {
            $makensisPath = $candidate
            break
        }
    }
}
if (-not $makensisPath) { throw "makensis.exe was not found after NSIS installation." }

$dist = Join-Path $repoRoot "dist"
if (-not (Test-Path $dist)) { New-Item -ItemType Directory -Force -Path $dist | Out-Null }
if (-not $OutputPath) { $OutputPath = Join-Path $dist ("ClashGO-v{0}-windows-setup.exe" -f $Version) }
if (Test-Path $OutputPath) { Remove-Item $OutputPath -Force }

$script = Join-Path $repoRoot "build\windows\installer-portable.nsi"
& $makensisPath "/DVERSION=$Version" "/DBUNDLE_DIR=$BundlePath" "/DOUTPUT_PATH=$OutputPath" $script
if ($LASTEXITCODE -ne 0) { throw "NSIS installer build failed with exit code $LASTEXITCODE" }
if (-not (Test-Path $OutputPath)) { throw "NSIS reported success but installer was not created at $OutputPath" }

Write-Host ""
Write-Host "INSTALLER COMPLETE"
Write-Host "Setup: $OutputPath"
