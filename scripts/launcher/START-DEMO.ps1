# SafePaw — PowerShell start/stop (alternative to start.bat)
# Preferred one-click menu: LAUNCH.bat (full / demo / stop / processes).
# This script supports the same modes for PowerShell users: no args = full, --demo = demo, --stop = shut down.

$ErrorActionPreference = "Stop"
$here = Resolve-Path (Join-Path (Split-Path -Parent $MyInvocation.MyCommand.Path) "..\..")
Set-Location $here

$mode = "full"
if ($args -contains "--demo") { $mode = "demo" }
if ($args -contains "--stop") {
    Write-Host "Stopping SafePaw services..." -ForegroundColor Yellow
    docker compose down 2>$null
    docker compose -f docker-compose.demo.yml down 2>$null
    Write-Host "[OK] All stopped." -ForegroundColor Green
    exit 0
}

if ($mode -eq "demo") {
    $chars = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789!@#$%^&*()-_=+"
    $demoPassword = -join (1..16 | ForEach-Object { $chars[(Get-Random -Minimum 0 -Maximum $chars.Length)] })
    $env:WIZARD_ADMIN_PASSWORD = $demoPassword
    Write-Host "Starting demo (SafePaw only, no API key)..." -ForegroundColor Cyan
    docker compose -f docker-compose.demo.yml up -d --build
} else {
    Write-Host "Starting full stack (SafePaw + OpenClaw)..." -ForegroundColor Cyan
    docker compose up -d --build
}

if ($LASTEXITCODE -ne 0) {
    Write-Host "`nDocker failed. Is Docker Desktop running?" -ForegroundColor Red
    Write-Host "Start Docker Desktop, wait until it says 'Running', then run this script again." -ForegroundColor Yellow
    exit 1
}

Write-Host "`nDone. Wait about 30-60 seconds for everything to be ready." -ForegroundColor Green
Write-Host "`nWizard:  http://localhost:3000" -ForegroundColor White
if ($mode -eq "demo") {
    Write-Host "Password: $demoPassword" -ForegroundColor Yellow
} else {
    Write-Host "Password: check .env WIZARD_ADMIN_PASSWORD or docker compose logs wizard" -ForegroundColor Gray
}
Write-Host "Gateway: http://localhost:8080/health" -ForegroundColor Gray
Write-Host "`nTo stop: .\START-DEMO.ps1 --stop  or use LAUNCH.bat [3]" -ForegroundColor Gray
