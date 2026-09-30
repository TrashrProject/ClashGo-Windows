param(
    [Parameter(Mandatory=$true)][string]$ZipPath,
    [Parameter(Mandatory=$true)][string]$InstallDir,
    [Parameter(Mandatory=$true)][string]$ExePath,
    [Parameter(Mandatory=$true)][int]$ParentPID
)

$ErrorActionPreference = "Stop"
$work = Join-Path $env:TEMP ("ClashGO-update-" + [Guid]::NewGuid().ToString("N"))
$backup = Join-Path $env:TEMP ("ClashGO-backup-" + [Guid]::NewGuid().ToString("N"))
$logPath = Join-Path $env:TEMP "ClashGO-update.log"
$backupReady = $false
$updateCommitted = $false

function Write-UpdateLog([string]$Message) {
    $line = "{0:o} {1}" -f (Get-Date), $Message
    Add-Content -LiteralPath $logPath -Value $line -Encoding UTF8 -ErrorAction SilentlyContinue
}

function Invoke-RobocopyChecked(
    [string]$Source,
    [string]$Destination,
    [string[]]$ExtraArgs = @()
) {
    New-Item -ItemType Directory -Force -Path $Destination | Out-Null
    $args = @($Source, $Destination, "/E", "/COPY:DAT", "/DCOPY:DAT", "/R:10", "/W:1", "/NFL", "/NDL", "/NJH", "/NJS", "/NP") + $ExtraArgs
    & robocopy.exe @args
    $rc = $LASTEXITCODE
    if ($rc -gt 7) {
        throw "robocopy failed from '$Source' to '$Destination' (exit code $rc)."
    }
}

function Restore-PreviousInstall {
    if (-not $backupReady -or -not (Test-Path -LiteralPath $backup)) {
        Write-UpdateLog "Rollback skipped: no completed backup is available."
        return $false
    }

    Write-UpdateLog "Rollback started from $backup"
    try {
        New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
        & robocopy.exe $backup $InstallDir /MIR /COPY:DAT /DCOPY:DAT /R:10 /W:1 /NFL /NDL /NJH /NJS /NP
        $rollbackRC = $LASTEXITCODE
        if ($rollbackRC -gt 7) {
            Write-UpdateLog "Rollback copy failed with exit code $rollbackRC"
            return $false
        }

        $oldExe = Join-Path $InstallDir "ClashGO.exe"
        if (-not (Test-Path -LiteralPath $oldExe)) {
            Write-UpdateLog "Rollback copy finished but ClashGO.exe is missing."
            return $false
        }

        Write-UpdateLog "Rollback completed. Relaunching previous ClashGO."
        Start-Process -FilePath $oldExe -WorkingDirectory $InstallDir
        return $true
    }
    catch {
        Write-UpdateLog ("Rollback exception: " + $_.Exception.Message)
        return $false
    }
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
    if ($newExe.Length -le 0) {
        throw "The update archive contains an empty ClashGO.exe."
    }

    $source = Split-Path -Parent $newExe.FullName
    Write-UpdateLog "Resolved update source: $source"

    if (Test-Path -LiteralPath $InstallDir) {
        Write-UpdateLog "Creating rollback snapshot: $backup"
        Invoke-RobocopyChecked -Source $InstallDir -Destination $backup
        $backupExe = Join-Path $backup "ClashGO.exe"
        if (-not (Test-Path -LiteralPath $backupExe)) {
            throw "Rollback snapshot is incomplete: ClashGO.exe is missing."
        }
        $backupReady = $true
        Write-UpdateLog "Rollback snapshot completed."
    }

    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    Invoke-RobocopyChecked -Source $source -Destination $InstallDir

    $targetExe = Join-Path $InstallDir "ClashGO.exe"
    if (-not (Test-Path -LiteralPath $targetExe)) {
        throw "Updated ClashGO.exe is missing after file replacement."
    }

    $info = Get-Item -LiteralPath $targetExe
    if ($info.Length -le 0) {
        throw "Updated ClashGO.exe is empty."
    }

    Write-UpdateLog "Update copied successfully. Relaunching $targetExe"
    $newProcess = Start-Process -FilePath $targetExe -WorkingDirectory $InstallDir -PassThru

    Start-Sleep -Seconds 4
    $newProcess.Refresh()
    if ($newProcess.HasExited) {
        throw "The updated ClashGO process exited immediately (exit code $($newProcess.ExitCode))."
    }

    $updateCommitted = $true
    Write-UpdateLog "Update committed: new ClashGO stayed alive through startup verification."
}
catch {
    $msg = $_.Exception.Message
    Write-UpdateLog "FAILED: $msg"

    $rolledBack = Restore-PreviousInstall
    if ($rolledBack) {
        $rollbackText = "The previous ClashGO version was restored and relaunched."
    }
    elseif ($backupReady) {
        $rollbackText = "Automatic rollback could not be completed. Backup: $backup"
    }
    else {
        $rollbackText = "No installed files were changed before the failure, or no rollback snapshot was available."
    }

    Write-UpdateLog "Rollback result: $rollbackText"
    Add-Type -AssemblyName PresentationFramework -ErrorAction SilentlyContinue
    try {
        $nl = [Environment]::NewLine
        $message = "ClashGO update failed. $msg" + $nl + $nl + $rollbackText + $nl + $nl + "Diagnostic log: $logPath"
        [System.Windows.MessageBox]::Show($message, "ClashGO Update") | Out-Null
    } catch {}
    exit 1
}
finally {
    Start-Sleep -Milliseconds 500
    Remove-Item -LiteralPath $work -Recurse -Force -ErrorAction SilentlyContinue

    if ($updateCommitted) {
        Remove-Item -LiteralPath $backup -Recurse -Force -ErrorAction SilentlyContinue
    }

    Remove-Item -LiteralPath $PSCommandPath -Force -ErrorAction SilentlyContinue
}
