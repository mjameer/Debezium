$ErrorActionPreference = "Continue"
$ROOT = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)

Write-Host ""
Write-Host "============================================" -ForegroundColor Cyan
Write-Host "  WAL CDC Replication POC" -ForegroundColor Cyan
Write-Host "============================================" -ForegroundColor Cyan

# ── Network ────────────────────────────────────────────
Write-Host "`n=== Creating shared network ===" -ForegroundColor Yellow
podman network create cdc-network 2>$null
Write-Host "  Done (or already exists)"

# ── 1. postgres1 ──────────────────────────────────────
Write-Host "`n=== 1. Starting postgres1 (source OME database) ===" -ForegroundColor Yellow
Push-Location "$ROOT\1-reader"
py -m podman_compose up -d
Pop-Location

Write-Host "  Waiting for postgres1..."
do {
    Start-Sleep -Seconds 3
    podman exec postgres1 pg_isready -U postgres -d omedb 2>$null | Out-Null
} until ($LASTEXITCODE -eq 0)
Write-Host "  postgres1 ready." -ForegroundColor Green

# ── 2. Kafka + Debezium ──────────────────────────────
Write-Host "`n=== 2. Starting Kafka + Debezium (CDC pipeline) ===" -ForegroundColor Yellow
Push-Location "$ROOT\2-kafka-debezium"
py -m podman_compose up -d
Pop-Location

Write-Host "  Waiting for Debezium (takes ~60-90s)..."
do {
    Start-Sleep -Seconds 5
    try {
        $resp = Invoke-WebRequest -Uri "http://localhost:8083/connectors" -UseBasicParsing -ErrorAction SilentlyContinue
    } catch {
        $resp = $null
    }
} until ($resp -and $resp.StatusCode -eq 200)
Write-Host "  Debezium ready." -ForegroundColor Green

# ── 3. postgres2 + writer ────────────────────────────
Write-Host "`n=== 3. Starting postgres2 + writer (Go consumer) ===" -ForegroundColor Yellow
Push-Location "$ROOT\3-writer"
py -m podman_compose up --build -d
Pop-Location

Write-Host ""
Write-Host "============================================" -ForegroundColor Green
Write-Host "  ALL STARTED" -ForegroundColor Green
Write-Host "============================================" -ForegroundColor Green
Write-Host ""
Write-Host "  Watch writer logs:" -ForegroundColor Cyan
Write-Host "    podman logs -f writer"
Write-Host ""
Write-Host "  Verify replication:" -ForegroundColor Cyan
Write-Host "    .\scripts\verify.ps1"
Write-Host ""
Write-Host "  Stop everything:" -ForegroundColor Cyan
Write-Host "    .\scripts\stop-all.ps1"
Write-Host "============================================"
