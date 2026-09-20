param(
    [Parameter(Mandatory=$true)][string]$ZipPath,
    [Parameter(Mandatory=$true)][string]$InstallDir,
    [Parameter(Mandatory=$true)][string]$ExePath,
    [Parameter(Mandatory=$true)][int]$ParentPID
)

$ErrorActionPreference = "Stop"
$work = Join-Path $env:TEMP ("ClashGO-update-" + [Guid]::NewGuid().ToString("N"))

try {
    while (Get-Process -Id $ParentPID -ErrorAction SilentlyContinue) {
        Start-Sleep -Milliseconds 250
    }

    New-Item -ItemType Directory -Force -Path $work | Out-Null
    Expand-Archive -LiteralPath $ZipPath -DestinationPath $work -Force

    $children = @(Get-ChildItem -LiteralPath $work)
    $source = $work
    if ($children.Count -eq 1 -and $children[0].PSIsContainer) {
        $source = $children[0].FullName
    }

    Get-ChildItem -LiteralPath $source -Force | ForEach-Object {
        Copy-Item -LiteralPath $_.FullName -Destination $InstallDir -Recurse -Force
    }

    Start-Process -FilePath $ExePath -WorkingDirectory $InstallDir
}
catch {
    $msg = $_.Exception.Message
    Add-Type -AssemblyName PresentationFramework -ErrorAction SilentlyContinue
    try { [System.Windows.MessageBox]::Show("ClashGO update failed: $msg", "ClashGO Update") | Out-Null } catch {}
    exit 1
}
finally {
    Start-Sleep -Milliseconds 500
    Remove-Item -LiteralPath $work -Recurse -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $PSCommandPath -Force -ErrorAction SilentlyContinue
}
