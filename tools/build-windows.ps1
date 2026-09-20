param([string]$Version = "0.6.0-windows-beta", [switch]$SkipSync, [switch]$SkipTests)

$ErrorActionPreference = "Stop"
$repoRoot = Split-Path -Parent $PSScriptRoot
Push-Location $repoRoot
try {
    if (-not $SkipSync -and -not (Test-Path ".\assets\templates\btn_attack.png")) { & ".\tools\sync-upstream-runtime.ps1" }
    if (-not (Test-Path ".\assets\templates\btn_attack.png")) { throw "Runtime templates are missing. Run tools\sync-upstream-runtime.ps1 first." }

    $opencvBin = "C:\opencv\build\install\x64\mingw\bin"
    if ($env:CLASHGO_OPENCV_BIN) { $opencvBin = $env:CLASHGO_OPENCV_BIN }
    if (-not (Test-Path $opencvBin)) { throw "OpenCV runtime was not found at $opencvBin. Run tools\setup-windows-dev.ps1 -InstallOpenCV." }
    # Prefer an explicit override, then the normal MSYS2 UCRT toolchain,
    # then any working MinGW compiler already available on PATH (including the
    # Chocolatey toolchain used by GitHub's Windows runner). The old hard-coded
    # MSYS2-only path made CI fail after every otherwise-green test/build.
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
    if (-not $mingwBin -or -not (Test-Path (Join-Path $mingwBin "gcc.exe")) -or -not (Test-Path (Join-Path $mingwBin "g++.exe"))) {
        throw "A usable MinGW gcc/g++ toolchain was not found. Install MSYS2 UCRT64 or set CLASHGO_MINGW_BIN."
    }

    # Wails' binding-generation step invokes the Go toolchain before the final
    # application compile. On Windows, if CGO is disabled (or GCC is not on
    # PATH), Go silently excludes GoCV's files that import "C" while keeping
    # generated *_string.go files. The resulting symptom is misleading errors
    # such as "undefined: MatType" / "undefined: CompareType".
    #
    # Make the Windows build self-contained instead of relying on whatever PATH
    # happened to be present in the interactive PowerShell session.
    $env:PATH = "$mingwBin;$opencvBin;$env:PATH"
    $env:CGO_ENABLED = "1"
    $env:CC = (Join-Path $mingwBin "gcc.exe")
    $env:CXX = (Join-Path $mingwBin "g++.exe")

$gocvTags = "customenv,gocv_specific_modules"
$env:CGO_CXXFLAGS = "--std=c++11 -DNDEBUG"
$env:CGO_CPPFLAGS = "-IC:/opencv/build/install/include"
$env:CGO_LDFLAGS = "-LC:/opencv/build/install/x64/mingw/lib -lopencv_core4130 -lopencv_imgproc4130 -lopencv_imgcodecs4130"

    if (-not (Get-Command wails.exe -ErrorAction SilentlyContinue)) {
        Write-Host "Installing Wails v2.12.0..."
        go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0
    }

    Write-Host "Installing frontend dependencies..."
    Push-Location ".\web"
    try {
        npm ci
        if ($LASTEXITCODE -ne 0) { throw "npm ci failed" }
    } finally { Pop-Location }

    if (-not $SkipTests) {
        Write-Host "Running focused tests..."
        go test -tags $gocvTags ./internal/updater ./internal/paths
        if ($LASTEXITCODE -ne 0) { throw "Go tests failed" }
    }

    $commit = (git rev-parse HEAD).Trim()
    $ldflags = "-X main.version=$Version -X main.commit=$commit"
    Write-Host "Building Wails Windows application..."
    wails build -clean -webview2 embed -o ClashGO.exe -tags $gocvTags -ldflags $ldflags
    if ($LASTEXITCODE -ne 0) { throw "Wails build failed" }

    $exe = ".\build\bin\ClashGO.exe"
    if (-not (Test-Path $exe)) {
        $candidate = Get-ChildItem ".\build\bin" -Filter "*.exe" | Select-Object -First 1
        if (-not $candidate) { throw "Wails did not produce an executable in build\bin" }
        $exe = $candidate.FullName
    }

    $distRoot = Join-Path $repoRoot "dist"
    $bundle = Join-Path $distRoot "ClashGO-Windows"
    if (Test-Path $bundle) { Remove-Item $bundle -Recurse -Force }
    New-Item -ItemType Directory -Force -Path $bundle | Out-Null
    New-Item -ItemType Directory -Force -Path (Join-Path $bundle "resources") | Out-Null
    Copy-Item $exe (Join-Path $bundle "ClashGO.exe") -Force
    Copy-Item ".\assets" (Join-Path $bundle "assets") -Recurse -Force
    Copy-Item ".\LICENSE" (Join-Path $bundle "LICENSE.txt") -Force
    Copy-Item ".\docs\WINDOWS_PORT.md" (Join-Path $bundle "WINDOWS_PORT.md") -Force
    Copy-Item ".\tools\windows-doctor.ps1" (Join-Path $bundle "windows-doctor.ps1") -Force
    Copy-Item ".\build\windows\install_update.ps1" (Join-Path $bundle "resources\install_update.ps1") -Force

    Write-Host "Copying OpenCV runtime DLLs..."
    Get-ChildItem $opencvBin -Filter "*.dll" | Copy-Item -Destination $bundle -Force
    $gxx = Get-Command g++.exe -ErrorAction SilentlyContinue
    if ($gxx) {
        $mingwBin = Split-Path $gxx.Source -Parent
        foreach ($dll in @("libgcc_s_seh-1.dll","libgcc_s_sjlj-1.dll","libstdc++-6.dll","libwinpthread-1.dll")) {
            $p = Join-Path $mingwBin $dll
            if (Test-Path $p) { Copy-Item $p $bundle -Force }
        }
    }

    $versionText = "ClashGO Windows`r`nVersion: $Version`r`nCommit: $commit`r`nBuilt: $(Get-Date -Format o)`r`n"
    Set-Content -Path (Join-Path $bundle "VERSION.txt") -Value $versionText -Encoding UTF8

    if (-not (Test-Path $distRoot)) { New-Item -ItemType Directory -Force -Path $distRoot | Out-Null }
    $zip = Join-Path $distRoot ("ClashGO-v{0}-windows.zip" -f $Version)
    if (Test-Path $zip) { Remove-Item $zip -Force }
    Compress-Archive -Path $bundle -DestinationPath $zip -CompressionLevel Optimal
    Write-Host ""
    Write-Host "BUILD COMPLETE"
    Write-Host "Folder: $bundle"
    Write-Host "ZIP:    $zip"
} finally { Pop-Location }
