$ErrorActionPreference = "Stop"
$repoRoot = Split-Path -Parent $PSScriptRoot
Push-Location $repoRoot
try {
    if (-not (Test-Path ".\assets\templates\btn_attack.png")) { & ".\tools\sync-upstream-runtime.ps1" }

    $opencvBin = if ($env:CLASHGO_OPENCV_BIN) { $env:CLASHGO_OPENCV_BIN } else { "C:\opencv\build\install\x64\mingw\bin" }
    if (-not (Test-Path $opencvBin)) {
        throw "OpenCV runtime was not found at $opencvBin. Run tools\setup-windows-dev.ps1 -InstallOpenCV."
    }

    # Wails generates Go bindings before starting the dev server. GoCV needs
    # CGO enabled during that generation step as well, otherwise Go excludes
    # the files defining MatType/CompareType and the generated *_string.go
    # files fail with misleading 'undefined' errors.
    $mingwBin = $null
    if ($env:CLASHGO_MINGW_BIN -and (Test-Path (Join-Path $env:CLASHGO_MINGW_BIN "g++.exe"))) {
        $mingwBin = $env:CLASHGO_MINGW_BIN
    }
    if (-not $mingwBin) {
        $msysCandidate = "C:\msys64\ucrt64\bin"
        if (Test-Path (Join-Path $msysCandidate "g++.exe")) {
            $mingwBin = $msysCandidate
        }
    }
    if (-not $mingwBin) {
        $pathGxx = Get-Command g++.exe -ErrorAction SilentlyContinue
        if ($pathGxx) {
            $mingwBin = Split-Path $pathGxx.Source -Parent
        }
    }
    if (-not $mingwBin) {
        $chocoCandidate = "C:\ProgramData\mingw64\mingw64\bin"
        if (Test-Path (Join-Path $chocoCandidate "g++.exe")) {
            $mingwBin = $chocoCandidate
        }
    }
    if (-not $mingwBin -or
        -not (Test-Path (Join-Path $mingwBin "gcc.exe")) -or
        -not (Test-Path (Join-Path $mingwBin "g++.exe"))) {
        throw "A usable MinGW gcc/g++ toolchain was not found. Install MSYS2 UCRT64 or set CLASHGO_MINGW_BIN."
    }

    $env:PATH = "$mingwBin;$opencvBin;$env:PATH"
    $env:CGO_ENABLED = "1"
    $env:CC = (Join-Path $mingwBin "gcc.exe")
    $env:CXX = (Join-Path $mingwBin "g++.exe")
    $env:CGO_CXXFLAGS = "--std=c++11 -DNDEBUG"
    $env:CGO_CPPFLAGS = "-IC:/opencv/build/install/include"
    $env:CGO_LDFLAGS = "-LC:/opencv/build/install/x64/mingw/lib -lopencv_core4130 -lopencv_imgproc4130 -lopencv_imgcodecs4130"
    $env:GOFLAGS = "-tags=customenv,gocv_specific_modules"

    Write-Host "Windows dev toolchain"
    Write-Host "---------------------"
    Write-Host "MinGW:  $mingwBin"
    Write-Host "OpenCV: $opencvBin"
    Write-Host "CGO:    $env:CGO_ENABLED"
    if (-not (Get-Command wails.exe -ErrorAction SilentlyContinue)) { go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0 }
    wails dev -tags customenv,gocv_specific_modules
} finally { Pop-Location }
