param(
    [Parameter(Mandatory=$true)][string]$ZipPath,
    [Parameter(Mandatory=$true)][string]$InstallDir,
    [Parameter(Mandatory=$true)][string]$ExePath,
    [Parameter(Mandatory=$true)][int]$ParentPID
)

$ErrorActionPreference = "Stop"
$work = Join-Path $env:TEMP ("ClashGO-update-" + [Guid]::NewGuid().ToString("N"))
$logPath = Join-Path $env:TEMP "ClashGO-update.log"

function Write-UpdateLog([string]$Message) {
    $line = "{0:o} {1}" -f (Get-Date), $Message
    Add-Content -LiteralPath $logPath -Value $line -Encoding UTF8 -ErrorAction SilentlyContinue
}

try {
    Write-UpdateLog "Updater started. ParentPID=$ParentPID InstallDir=$InstallDir Zip=$ZipPath"

    $deadline = (Get-Date).AddSeconds(60)
    while ((Get-Process -Id $ParentPID -ErrorAction SilentlyContinue) -and (Get-Date) -lt $deadline) {
        Start-Sleep -Milliseconds 250
    }
    if (Get-Process -Id $ParentPID -ErrorAction SilentlyContinue) {
        throw "ClashGO did not exit within 60 seconds."
    }

    if (-not (Test-Path -LiteralPath $ZipPath)) {
        throw "Downloaded update archive was not found: $ZipPath"
    }

    New-Item -ItemType Directory -Force -Path $work | Out-Null
    Expand-Archive -LiteralPath $ZipPath -DestinationPath $work -Force

    $newExe = Get-ChildItem -LiteralPath $work -Filter "ClashGO.exe" -File -Recurse | Select-Object -First 1
    if (-not $newExe) {
        throw "The update archive does not contain ClashGO.exe."
    }
    $source = Split-Path -Parent $newExe.FullName
    Write-UpdateLog "Resolved update source: $source"

    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null

    & robocopy.exe $source $InstallDir /E /COPY:DAT /DCOPY:DAT /R:20 /W:1 /NFL /NDL /NJH /NJS /NP
    $rc = $LASTEXITCODE
    if ($rc -gt 7) {
        throw "File replacement failed (robocopy exit code $rc)."
    }

    $targetExe = Join-Path $InstallDir "ClashGO.exe"
    if (-not (Test-Path -LiteralPath $targetExe)) {
        throw "Updated ClashGO.exe is missing after file replacement."
    }

    $info = Get-Item -LiteralPath $targetExe
    if ($info.Length -le 0) {
        throw "Updated ClashGO.exe is empty."
    }

    Write-UpdateLog "Update copied successfully. Relaunching $targetExe"
    Start-Process -FilePath $targetExe -WorkingDirectory $InstallDir
}
catch {
    $msg = $_.Exception.Message
    Write-UpdateLog "FAILED: $msg"
    Add-Type -AssemblyName PresentationFramework -ErrorAction SilentlyContinue
    try {
        [System.Windows.MessageBox]::Show("ClashGO update failed. $msg. Diagnostic log: $logPath", "ClashGO Update") | Out-Null
    } catch {}
    exit 1
}
finally {
    Start-Sleep -Milliseconds 500
    Remove-Item -LiteralPath $work -Recurse -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $PSCommandPath -Force -ErrorAction SilentlyContinue
}
