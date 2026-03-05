# InstallerClaw — Run locally (no Docker)
# 3 Go processes: mockbackend, gateway, wizard
# Press ENTER to stop everything

$ErrorActionPreference = "Stop"
$here = Split-Path -Parent $MyInvocation.MyCommand.Path

Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  InstallerClaw — Local Mode (no Docker)" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

# Build if missing
$mockExe = Join-Path $here "services\mockbackend\mockbackend.exe"
$gwExe   = Join-Path $here "services\gateway\gateway.exe"
$wizExe  = Join-Path $here "services\wizard\wizard.exe"

if (-not (Test-Path $mockExe)) {
    Write-Host "Building mockbackend..." -ForegroundColor Yellow
    Push-Location (Join-Path $here "services\mockbackend"); go build -o mockbackend.exe .; Pop-Location
}
if (-not (Test-Path $gwExe)) {
    Write-Host "Building gateway..." -ForegroundColor Yellow
    Push-Location (Join-Path $here "services\gateway"); go build -o gateway.exe .; Pop-Location
}
if (-not (Test-Path $wizExe)) {
    Write-Host "Building wizard..." -ForegroundColor Yellow
    Push-Location (Join-Path $here "services\wizard"); go build -o wizard.exe ./cmd/wizard; Pop-Location
}

# Start mockbackend
Write-Host "[1/3] Starting mockbackend on :18789" -ForegroundColor Green
$mock = Start-Process -FilePath $mockExe -WorkingDirectory (Split-Path $mockExe) -PassThru -WindowStyle Hidden
Start-Sleep -Seconds 1

# Start gateway
Write-Host "[2/3] Starting gateway on :8080" -ForegroundColor Green
$gw = Start-Process -FilePath $gwExe -WorkingDirectory (Split-Path $gwExe) -PassThru -WindowStyle Hidden -ArgumentList @() -EnvironmentVariable @{
} 2>$null
# Set env before starting
$env:PROXY_TARGET = "http://localhost:18789"
$env:AUTH_ENABLED = "false"
$env:GATEWAY_PORT = "8080"
$env:LOG_FORMAT = "text"
$env:RATE_LIMIT = "60"
$gw = Start-Process -FilePath $gwExe -WorkingDirectory (Split-Path $gwExe) -PassThru -WindowStyle Hidden
Start-Sleep -Seconds 1

# Start wizard
Write-Host "[3/3] Starting wizard on :3000" -ForegroundColor Green
$env:WIZARD_ADMIN_PASSWORD = "DemoPassword123!"
$env:WIZARD_PORT = "3000"
$env:SECURE_COOKIES = "false"
$env:OPENCLAW_HEALTH_URL = "http://localhost:18789/health"
$env:DOCKER_HOST = "npipe:////./pipe/docker_engine"
$wiz = Start-Process -FilePath $wizExe -WorkingDirectory (Split-Path $wizExe) -PassThru -WindowStyle Hidden
Start-Sleep -Seconds 2

Write-Host ""
Write-Host "========================================" -ForegroundColor Green
Write-Host "  ALL RUNNING" -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Green
Write-Host ""
Write-Host "  Wizard:   http://localhost:3000" -ForegroundColor White
Write-Host "  Password: DemoPassword123!" -ForegroundColor Yellow
Write-Host "  Gateway:  http://localhost:8080/health" -ForegroundColor White
Write-Host "  Backend:  http://localhost:18789" -ForegroundColor White
Write-Host ""
Write-Host "  Press ENTER to stop everything." -ForegroundColor Gray
Write-Host ""

Start-Process "http://localhost:3000"

Read-Host | Out-Null

Write-Host "Stopping..." -ForegroundColor Yellow
foreach ($p in @($mock, $gw, $wiz)) {
    if ($p -and -not $p.HasExited) {
        Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue
    }
}
Write-Host "Stopped. All clean." -ForegroundColor Green
