param([string]$UpstreamCommit = "4fa82f2cdc3ac4897b1cab0fff29565cd16deef6")

$ErrorActionPreference = "Stop"
$repoRoot = Split-Path -Parent $PSScriptRoot
Push-Location $repoRoot
try {
    $remotes = git remote
    if ($remotes -notcontains "upstream") { git remote add upstream https://github.com/DSargent21/ClashGo.git }
    Write-Host "Fetching audited ClashGO runtime assets..."
    git fetch upstream $UpstreamCommit --depth=1
    git checkout $UpstreamCommit -- assets build/appicon.png internal/game/testdata
    if ($LASTEXITCODE -ne 0) { throw "Unable to sync upstream runtime assets" }
    Write-Host "Runtime assets synchronized from $UpstreamCommit."
} finally { Pop-Location }
