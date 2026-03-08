# InstallerClaw / SafePaw — Start demo (double-click or run in PowerShell)
# Make sure Docker Desktop is running first!

$ErrorActionPreference = "Stop"
$here = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $here

Write-Host "Starting InstallerClaw stack..." -ForegroundColor Cyan
docker compose up -d
if ($LASTEXITCODE -ne 0) {
    Write-Host "`nDocker failed. Is Docker Desktop running?" -ForegroundColor Red
    Write-Host "Start Docker Desktop, wait until it says 'Running', then run this script again." -ForegroundColor Yellow
    exit 1
}

Write-Host "`nDone. Wait about 30-60 seconds for everything to be ready." -ForegroundColor Green
Write-Host "`nThen open in your browser:  http://localhost:3000" -ForegroundColor White
Write-Host "Login password: DemoPassword123!" -ForegroundColor Yellow
Write-Host "`nGateway (after login): http://localhost:8080/health" -ForegroundColor Gray
Write-Host "`nTo stop later: docker compose down" -ForegroundColor Gray
