param(
    [string]$Server = "http://127.0.0.1:8787",
    [string]$Listen = "127.0.0.1:8787",
    [switch]$ResetData
)

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$dataDir = Join-Path $repoRoot "data"
$dataPath = Join-Path $dataDir "clashgo-control-local.json"
$secretPath = Join-Path $dataDir "clashgo-control-local-admin.txt"
$serverScript = Join-Path $PSScriptRoot "run-control-local.ps1"

New-Item -ItemType Directory -Force -Path $dataDir | Out-Null

if ($ResetData) {
    Remove-Item -LiteralPath $dataPath -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $secretPath -Force -ErrorAction SilentlyContinue
}

$Server = $Server.Trim().TrimEnd("/")

function Test-ControlHealth {
    try {
        $result = Invoke-RestMethod -Uri "$Server/healthz" -Method Get -TimeoutSec 2
        return [bool]$result.ok
    } catch {
        return $false
    }
}

if (-not (Test-ControlHealth)) {
    Write-Host "Démarrage du serveur ClashGO local..." -ForegroundColor Cyan
    $args = @(
        "-NoExit",
        "-ExecutionPolicy", "Bypass",
        "-File", ('"' + $serverScript + '"'),
        "-Listen", $Listen
    )
    Start-Process powershell.exe -ArgumentList ($args -join " ") | Out-Null

    $ready = $false
    for ($i = 0; $i -lt 30; $i++) {
        Start-Sleep -Milliseconds 500
        if (Test-ControlHealth) {
            $ready = $true
            break
        }
    }

    if (-not $ready) {
        throw "Le serveur local ne répond pas sur $Server après 15 secondes."
    }
}

if (-not (Test-Path $secretPath)) {
    throw "Clé admin locale introuvable : $secretPath"
}

$adminKey = (Get-Content -LiteralPath $secretPath -Raw).Trim()
if (-not $adminKey) {
    throw "La clé admin locale est vide."
}

$headers = @{
    "X-ClashGO-Admin-Key" = $adminKey
    "Content-Type" = "application/json"
}

$payload = @{
    role = "admin"
    plan = "lifetime"
    count = 1
    customer_name = "Admin local"
    customer_notes = "Licence de test locale ClashGO"
    payment_status = "offered"
    amount_cents = 0
} | ConvertTo-Json -Compress

$result = Invoke-RestMethod -Uri "$Server/v1/admin/licenses" -Method Post -Headers $headers -Body $payload -TimeoutSec 10
$key = @($result.licenses)[0]

if (-not $key) {
    throw "Aucune licence Admin n'a été retournée."
}

try {
    Set-Clipboard -Value $key
    $clipboard = $true
} catch {
    $clipboard = $false
}

Write-Host ""
Write-Host "ClashGO · environnement licence local prêt" -ForegroundColor Green
Write-Host "===========================================" -ForegroundColor Green
Write-Host ""
Write-Host "Serveur : $Server" -ForegroundColor Cyan
Write-Host "Licence ADMIN : $key" -ForegroundColor Yellow
if ($clipboard) {
    Write-Host "La licence a été copiée dans le presse-papiers." -ForegroundColor Green
}
Write-Host ""
Write-Host "Dans la beta ClashGO :" -ForegroundColor White
Write-Host "1. Paramètres > Diagnostic > Serveur de licences ClashGO" -ForegroundColor White
Write-Host "2. Entre : $Server" -ForegroundColor White
Write-Host "3. Enregistre l'adresse (aucun redémarrage nécessaire)." -ForegroundColor White
Write-Host "4. Ouvre Mon ClashGO et active la licence ADMIN ci-dessus." -ForegroundColor White
Write-Host "5. Le menu Administration doit apparaître." -ForegroundColor White
Write-Host ""
Write-Host "Pour repartir de zéro plus tard :" -ForegroundColor DarkGray
Write-Host "  .\tools\start-local-license-test.ps1 -ResetData" -ForegroundColor DarkGray
Write-Host ""
