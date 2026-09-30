param(
    [string]$Listen = "127.0.0.1:8787",
    [switch]$ResetData
)

$ErrorActionPreference = "Stop"

$bundleRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
$binary = Join-Path $bundleRoot "clashgo-control-local.exe"
$dataDir = Join-Path $bundleRoot "data"
$dataPath = Join-Path $dataDir "clashgo-control-local.json"
$secretPath = Join-Path $dataDir "clashgo-control-local-admin.txt"

if (-not (Test-Path $binary)) {
    throw "Serveur local introuvable : $binary"
}

New-Item -ItemType Directory -Force -Path $dataDir | Out-Null
if ($ResetData) {
    Remove-Item -LiteralPath $dataPath -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $secretPath -Force -ErrorAction SilentlyContinue
}

if (-not (Test-Path $secretPath)) {
    $bytes = New-Object byte[] 32
    [System.Security.Cryptography.RandomNumberGenerator]::Fill($bytes)
    $adminKey = ([Convert]::ToHexString($bytes)).ToLowerInvariant()
    Set-Content -LiteralPath $secretPath -Value $adminKey -Encoding ascii -NoNewline
} else {
    $adminKey = (Get-Content -LiteralPath $secretPath -Raw).Trim()
}

$env:CLASHGO_ACCOUNT_LISTEN = $Listen
$env:CLASHGO_CONTROL_DATA = $dataPath
$env:CLASHGO_ADMIN_KEY = $adminKey
Remove-Item Env:COC_API_KEY -ErrorAction SilentlyContinue

$hostPart = $Listen
if ($hostPart.StartsWith(":")) { $hostPart = "127.0.0.1$hostPart" }
$server = "http://$hostPart"

Write-Host ""
Write-Host "ClashGO Control Local Portable" -ForegroundColor Cyan
Write-Host "==============================" -ForegroundColor Cyan
Write-Host "Serveur : $server" -ForegroundColor Green
Write-Host "Données : $dataPath"
Write-Host ""

$process = Start-Process -FilePath $binary -PassThru

try {
    $ready = $false
    for ($i = 0; $i -lt 30; $i++) {
        Start-Sleep -Milliseconds 500
        try {
            $health = Invoke-RestMethod -Uri "$server/healthz" -Method Get -TimeoutSec 2
            if ($health.ok) {
                $ready = $true
                break
            }
        } catch {}
    }

    if (-not $ready) {
        throw "Le serveur local ne répond pas après 15 secondes."
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
        customer_notes = "Licence Admin du bundle local ClashGO"
        payment_status = "offered"
        amount_cents = 0
    } | ConvertTo-Json -Compress

    $result = Invoke-RestMethod -Uri "$server/v1/admin/licenses" -Method Post -Headers $headers -Body $payload -TimeoutSec 10
    $key = @($result.licenses)[0]
    if (-not $key) {
        throw "Aucune licence Admin retournée."
    }

    try { Set-Clipboard -Value $key } catch {}

    Write-Host ""
    Write-Host "LICENCE ADMIN LOCALE" -ForegroundColor Yellow
    Write-Host $key -ForegroundColor Yellow
    Write-Host ""
    Write-Host "La clé est copiée dans le presse-papiers si Windows l'autorise." -ForegroundColor Green
    Write-Host "Dans ClashGO > Mon ClashGO > Mode test bêta :" -ForegroundColor White
    Write-Host "  Serveur : $server" -ForegroundColor White
    Write-Host "Puis active la licence Admin ci-dessus." -ForegroundColor White
    Write-Host ""
    Write-Host "Cette fenêtre garde le serveur actif. Ctrl+C pour arrêter." -ForegroundColor DarkGray

    while (-not $process.HasExited) {
        Start-Sleep -Seconds 1
        $process.Refresh()
    }
} finally {
    if (-not $process.HasExited) {
        Stop-Process -Id $process.Id -Force -ErrorAction SilentlyContinue
    }
}
