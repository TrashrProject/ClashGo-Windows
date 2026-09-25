param(
    [string]$Version = "0.6.7-windows-beta",
    [string]$Notes = "Test updater 0.6.6 vers 0.6.7 : detection, telechargement, verification, installation et redemarrage automatique."
)

$ErrorActionPreference = "Stop"
$repoRoot = Split-Path -Parent $PSScriptRoot
Set-Location $repoRoot

Write-Host ""
Write-Host "=== Publication mise a jour ClashGO ==="
Write-Host "Version: $Version"

$gh = Get-Command gh.exe -ErrorAction SilentlyContinue
if (-not $gh) {
    $winget = Get-Command winget.exe -ErrorAction SilentlyContinue
    if (-not $winget) {
        throw "GitHub CLI (gh) manque et winget est indisponible. Installe GitHub CLI puis relance."
    }
    Write-Host "Installation automatique de GitHub CLI..."
    & $winget.Source install --id GitHub.cli -e --silent --accept-package-agreements --accept-source-agreements
    if ($LASTEXITCODE -ne 0) { throw "Installation de GitHub CLI impossible." }
    $env:Path = "$env:ProgramFiles\GitHub CLI;$env:Path"
    $gh = Get-Command gh.exe -ErrorAction SilentlyContinue
}
if (-not $gh) { throw "gh.exe introuvable apres installation." }

& $gh.Source auth status
if ($LASTEXITCODE -ne 0) {
    throw "GitHub CLI n'est pas connecte. Lance 'gh auth login' une fois puis relance."
}

# Build the distributable installer + portable zip.
& (Join-Path $repoRoot "tools\make-friend-installer.ps1") -Version $Version
if ($LASTEXITCODE -ne 0) { throw "Build package ami impossible." }

$dist = Join-Path $repoRoot "dist"
$zip = Join-Path $dist ("ClashGO-v{0}-windows.zip" -f $Version)
$setup = Join-Path $dist ("ClashGO-v{0}-windows-setup.exe" -f $Version)
if (-not (Test-Path $zip)) { throw "ZIP de release introuvable: $zip" }
if (-not (Test-Path $setup)) { throw "Setup de release introuvable: $setup" }

$zipHash = (Get-FileHash -LiteralPath $zip -Algorithm SHA256).Hash.ToLowerInvariant()
$setupHash = (Get-FileHash -LiteralPath $setup -Algorithm SHA256).Hash.ToLowerInvariant()

@(
    "$zipHash  $([IO.Path]::GetFileName($zip))",
    "$setupHash  $([IO.Path]::GetFileName($setup))"
) | Set-Content -LiteralPath (Join-Path $dist "SHA256SUMS.txt") -Encoding ASCII

$assetName = [IO.Path]::GetFileName($zip)
$assetUrl = "https://github.com/TrashrProject/ClashGo-Windows/releases/download/v$Version/$assetName"
$manifest = [ordered]@{
    version = $Version
    release_date = (Get-Date).ToUniversalTime().ToString("o")
    notes = $Notes
    min_supported = "0.6.0-windows-beta"
    platforms = [ordered]@{
        windows = [ordered]@{
            asset_name = $assetName
            asset_url = $assetUrl
            size = (Get-Item $zip).Length
            sha256 = $zipHash
        }
    }
}
$manifestJson = $manifest | ConvertTo-Json -Depth 6
$utf8NoBom = New-Object System.Text.UTF8Encoding($false)
$latestPath = Join-Path $dist "latest.json"
[System.IO.File]::WriteAllText($latestPath, $manifestJson, $utf8NoBom)

$tag = "v$Version"

# gh writes "release not found" to stderr for a perfectly normal missing tag.
# With ErrorActionPreference=Stop, PowerShell 5 turns that stderr line into a
# terminating NativeCommandError before we can inspect $LASTEXITCODE. Probe
# existence through cmd.exe so a missing release is treated as "create it".
$ghPath = $gh.Source
$probe = '"{0}" release view "{1}" --repo TrashrProject/ClashGo-Windows >nul 2>nul' -f $ghPath, $tag
cmd.exe /d /s /c $probe
$exists = ($LASTEXITCODE -eq 0)

if ($exists) {
    Write-Host "Release $tag deja presente : remplacement des fichiers..."
    & $gh.Source release upload $tag $zip $setup $latestPath (Join-Path $dist "SHA256SUMS.txt") --clobber --repo TrashrProject/ClashGo-Windows
    if ($LASTEXITCODE -ne 0) { throw "Upload des fichiers de release impossible." }
    & $gh.Source release edit $tag --title "ClashGO Windows v$Version" --notes $Notes --repo TrashrProject/ClashGo-Windows
} else {
    Write-Host "Creation de la release $tag..."
    & $gh.Source release create $tag $zip $setup $latestPath (Join-Path $dist "SHA256SUMS.txt") --repo TrashrProject/ClashGo-Windows --target feature/windows-runtime-supervisor --title "ClashGO Windows v$Version" --notes $Notes
    if ($LASTEXITCODE -ne 0) { throw "Creation de la release GitHub impossible." }
}

Write-Host ""
Write-Host "MISE A JOUR PUBLIEE"
Write-Host "Les installations 0.6.0+ detecteront automatiquement v$Version."
Write-Host "Ton ami pourra cliquer sur 'Mettre a jour' puis ClashGO se relancera tout seul."
