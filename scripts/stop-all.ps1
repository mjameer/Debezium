$ErrorActionPreference = "Continue"
$ROOT = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)

Write-Host "Stopping 3-writer..." -ForegroundColor Yellow
Push-Location "$ROOT\3-writer"
py -m podman_compose down -v 2>$null
Pop-Location

Write-Host "Stopping 2-kafka-debezium..." -ForegroundColor Yellow
Push-Location "$ROOT\2-kafka-debezium"
py -m podman_compose down -v 2>$null
Pop-Location

Write-Host "Stopping 1-reader..." -ForegroundColor Yellow
Push-Location "$ROOT\1-reader"
py -m podman_compose down -v 2>$null
Pop-Location

Write-Host "Removing network..." -ForegroundColor Yellow
podman network rm cdc-network 2>$null

Write-Host "Done." -ForegroundColor Green
