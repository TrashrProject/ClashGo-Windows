param(
    [string]$Listen = "127.0.0.1:8787",
    [string]$DataPath = "",
    [switch]$RegenerateAdminKey
)

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
if (-not $DataPath) {
    $DataPath = Join-Path $repoRoot "data\clashgo-control-local.json"
}
$secretPath = Join-Path (Split-Path -Parent $DataPath) "clashgo-control-local-admin.txt"

New-Item -ItemType Directory -Force -Path (Split-Path -Parent $DataPath) | Out-Null

if ($RegenerateAdminKey -or -not (Test-Path $secretPath)) {
    $bytes = New-Object byte[] 32
    $rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
    try {
        $rng.GetBytes($bytes)
    } finally {
        $rng.Dispose()
    }
    # Compatible with Windows PowerShell / .NET Framework where
    # RandomNumberGenerator.Fill and Convert.ToHexString are unavailable.
    $adminKey = ([System.BitConverter]::ToString($bytes)).Replace("-", "").ToLowerInvariant()
    Set-Content -LiteralPath $secretPath -Value $adminKey -Encoding ascii -NoNewline
} else {
    $adminKey = (Get-Content -LiteralPath $secretPath -Raw).Trim()
}

$env:CLASHGO_ACCOUNT_LISTEN = $Listen
$env:CLASHGO_CONTROL_DATA = $DataPath
$env:CLASHGO_ADMIN_KEY = $adminKey
Remove-Item Env:COC_API_KEY -ErrorAction SilentlyContinue

$hostPart = $Listen
if ($hostPart.StartsWith(":")) {
    $hostPart = "127.0.0.1$hostPart"
}
$url = "http://$hostPart"

Write-Host ""
Write-Host "ClashGO Control Local" -ForegroundColor Cyan
Write-Host "=====================" -ForegroundColor Cyan
Write-Host "URL licences : $url" -ForegroundColor Green
Write-Host "Données       : $DataPath"
Write-Host "Clé admin     : $secretPath"
Write-Host ""
Write-Host "Mode contrôle uniquement : la clé API Clash n'est pas requise." -ForegroundColor Yellow
Write-Host "Dans ClashGO : Paramètres > Général > Serveur ClashGO > colle $url" -ForegroundColor Yellow
Write-Host ""
Write-Host "Ctrl+C pour arrêter le serveur." -ForegroundColor DarkGray
Write-Host ""

Push-Location $repoRoot
try {
    go run ./cmd/account_proxy
} finally {
    Pop-Location
}
