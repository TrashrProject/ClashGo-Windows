param([switch]$InstallOpenCV, [switch]$InstallWails = $true)

$ErrorActionPreference = "Stop"
function Need([string]$Command, [string]$Hint) {
    if (-not (Get-Command $Command -ErrorAction SilentlyContinue)) { throw "$Command is missing. $Hint" }
}
Need "git.exe" "Install Git for Windows."
Need "go.exe" "Install Go 1.25+."
Need "node.exe" "Install Node.js 20+."
Need "npm.cmd" "Install Node.js/npm."

if ($InstallWails -and -not (Get-Command wails.exe -ErrorAction SilentlyContinue)) {
    Write-Host "Installing Wails v2.12.0..."
    go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0
}

$opencvHeader = "C:\opencv\build\install\include\opencv2\core.hpp"
if (-not (Test-Path $opencvHeader)) {
    if (-not $InstallOpenCV) {
        Write-Warning "OpenCV 4.13.0 is not installed at C:\opencv."
        Write-Host "Run again with -InstallOpenCV to build it using GoCV official Windows scripts."
    } else {
        Need "cmake.exe" "Install CMake and add it to PATH."
        Need "g++.exe" "Install MinGW-w64 and add it to PATH."
        $temp = Join-Path $env:TEMP "gocv-clashgo-setup"
        if (Test-Path $temp) { Remove-Item $temp -Recurse -Force }
        git clone --depth 1 --branch v0.43.0 https://github.com/hybridgroup/gocv.git $temp
        Push-Location $temp
        try {
            & .\win_download_opencv.cmd
            if ($LASTEXITCODE -ne 0) { throw "OpenCV download failed" }

            # ClashGO does not need OpenCV's performance-test binaries. Skipping
            # them shortens the one-time native build, and CMake's standard
            # parallel-level environment variable lets MinGW use every core.
            (Get-Content .\win_build_opencv.cmd -Raw).Replace("-DBUILD_PERF_TESTS=ON", "-DBUILD_PERF_TESTS=OFF -DBUILD_LIST=core,imgproc,imgcodecs") |
                Set-Content .\win_build_opencv.cmd -Encoding ASCII
            $env:CMAKE_BUILD_PARALLEL_LEVEL = [Environment]::ProcessorCount
            Write-Host "Building OpenCV with $env:CMAKE_BUILD_PARALLEL_LEVEL parallel jobs..."

            & .\win_build_opencv.cmd
            if ($LASTEXITCODE -ne 0) { throw "OpenCV build failed" }
        } finally { Pop-Location }
    }
}

$opencvBin = "C:\opencv\build\install\x64\mingw\bin"
if (Test-Path $opencvBin) { $env:PATH = "$opencvBin;$env:PATH" }
Write-Host ""
Write-Host "Environment status"
Write-Host "------------------"
go version
node --version
npm --version
if (Get-Command wails.exe -ErrorAction SilentlyContinue) { wails version }
if (Test-Path $opencvHeader) { Write-Host "OpenCV: OK ($opencvHeader)" } else { Write-Host "OpenCV: MISSING" }
