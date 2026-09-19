$ErrorActionPreference = "Stop"
$repoRoot = Split-Path -Parent $PSScriptRoot
Push-Location $repoRoot
try {
    if (-not (Test-Path ".\assets\templates\btn_attack.png")) { & ".\tools\sync-upstream-runtime.ps1" }
    $opencvBin = if ($env:CLASHGO_OPENCV_BIN) { $env:CLASHGO_OPENCV_BIN } else { "C:\opencv\build\install\x64\mingw\bin" }
    if (Test-Path $opencvBin) { $env:PATH = "$opencvBin;$env:PATH" }
    if (-not (Get-Command wails.exe -ErrorAction SilentlyContinue)) { go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0 }
    wails dev
} finally { Pop-Location }
