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
    $env:PATH = "$opencvBin;$env:PATH"

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
        go test ./internal/updater ./internal/paths
        if ($LASTEXITCODE -ne 0) { throw "Go tests failed" }
    }

    $commit = (git rev-parse HEAD).Trim()
    $ldflags = "-X main.version=$Version -X main.commit=$commit"
    Write-Host "Building Wails Windows application..."
    wails build -clean -webview2 embed -o ClashGO.exe -ldflags $ldflags
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
    Copy-Item ".\docs\WINDOWS_QUICKSTART.md" (Join-Path $bundle "QUICKSTART.md") -Force
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
