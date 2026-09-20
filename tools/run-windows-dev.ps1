$ErrorActionPreference = "Stop"
$repoRoot = Split-Path -Parent $PSScriptRoot
Push-Location $repoRoot
try {
    if (-not (Test-Path ".\assets\templates\btn_attack.png")) { & ".\tools\sync-upstream-runtime.ps1" }
    $opencvBin = if ($env:CLASHGO_OPENCV_BIN) { $env:CLASHGO_OPENCV_BIN } else { "C:\opencv\build\install\x64\mingw\bin" }
    if (Test-Path $opencvBin) { $env:PATH = "$opencvBin;$env:PATH" }
$env:CGO_CXXFLAGS = "--std=c++11 -DNDEBUG"
$env:CGO_CPPFLAGS = "-IC:/opencv/build/install/include"
$env:CGO_LDFLAGS = "-LC:/opencv/build/install/x64/mingw/lib -lopencv_core4130 -lopencv_imgproc4130 -lopencv_imgcodecs4130"
$env:GOFLAGS = "-tags=customenv,gocv_specific_modules"
    if (-not (Get-Command wails.exe -ErrorAction SilentlyContinue)) { go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0 }
    wails dev -tags customenv,gocv_specific_modules
} finally { Pop-Location }
