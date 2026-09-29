param(
    [string]$Server = "http://127.0.0.1:8787",
    [string]$DataPath = ""
)

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
if (-not $DataPath) {
    $DataPath = Join-Path $repoRoot "data\clashgo-control-local.json"
}
$secretPath = Join-Path (Split-Path -Parent $DataPath) "clashgo-control-local-admin.txt"

if (-not (Test-Path $secretPath)) {
    throw "Clé admin locale introuvable. Lance d'abord .\tools\run-control-local.ps1"
}

$adminKey = (Get-Content -LiteralPath $secretPath -Raw).Trim()
if (-not $adminKey) {
    throw "La clé admin locale est vide."
}

$Server = $Server.Trim().TrimEnd("/")
$headers = @{
    "X-ClashGO-Admin-Key" = $adminKey
    "Content-Type" = "application/json"
}

try {
    $health = Invoke-RestMethod -Uri "$Server/healthz" -Method Get -TimeoutSec 5
} catch {
    throw "Serveur ClashGO local inaccessible sur $Server. Lance d'abord run-control-local.ps1 dans une autre fenêtre PowerShell."
}

$payload = @{
    role = "admin"
    plan = "lifetime"
    count = 1
} | ConvertTo-Json -Compress

$result = Invoke-RestMethod -Uri "$Server/v1/admin/licenses" -Method Post -Headers $headers -Body $payload -TimeoutSec 10
$key = @($result.licenses)[0]

if (-not $key) {
    throw "Le serveur n'a retourné aucune licence."
}

Write-Host ""
Write-Host "Licence ADMIN locale créée" -ForegroundColor Green
Write-Host "===========================" -ForegroundColor Green
Write-Host ""
Write-Host $key -ForegroundColor Cyan
Write-Host ""
Write-Host "Dans ClashGO :" -ForegroundColor Yellow
Write-Host "1. Paramètres > Général > Serveur ClashGO : $Server"
Write-Host "2. Enregistre l'adresse."
Write-Host "3. Active ClashGO avec la clé ADMIN ci-dessus."
Write-Host "4. Le menu Administration apparaîtra automatiquement."
Write-Host ""
Write-Host "Cette clé complète n'est pas enregistrée par le serveur, copie-la maintenant." -ForegroundColor DarkYellow
