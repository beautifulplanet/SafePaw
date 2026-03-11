@echo off
title SafePaw — One-Command Start
cd /d "%~dp0"

echo.
echo   SafePaw — Starting up...
echo.

:: ── Check Docker ────────────────────────────────────────────
docker info >nul 2>&1
if errorlevel 1 (
    echo [ERROR] Docker is not running.
    echo   1. Install Docker Desktop: https://docs.docker.com/get-docker/
    echo   2. Start Docker Desktop and wait until it says "Running"
    echo   3. Run this script again
    echo.
    pause
    exit /b 1
)
echo [OK] Docker running

:: ── Check for --demo flag ───────────────────────────────────
set MODE=full
if "%~1"=="--demo" set MODE=demo
if "%~1"=="--stop" (
    echo Stopping SafePaw services...
    docker compose down 2>nul
    docker compose -f docker-compose.demo.yml down 2>nul
    echo [OK] All services stopped.
    pause
    exit /b 0
)

:: ── Generate .env if missing ────────────────────────────────
if "%MODE%"=="full" (
    if not exist .env (
        echo Generating .env with secure defaults...
        copy .env.example .env >nul

        :: Generate secrets using PowerShell (available on all modern Windows)
        for /f "usebackq delims=" %%i in (`powershell -NoProfile -Command "[Convert]::ToBase64String((1..24 | ForEach-Object { Get-Random -Max 256 }) -as [byte[]])"`) do set REDIS_PW=%%i
        for /f "usebackq delims=" %%i in (`powershell -NoProfile -Command "[Convert]::ToBase64String((1..24 | ForEach-Object { Get-Random -Max 256 }) -as [byte[]])"`) do set PG_PW=%%i
        for /f "usebackq delims=" %%i in (`powershell -NoProfile -Command "[Convert]::ToBase64String((1..48 | ForEach-Object { Get-Random -Max 256 }) -as [byte[]])"`) do set AUTH_SEC=%%i
        for /f "usebackq delims=" %%i in (`powershell -NoProfile -Command "-join((1..14) | ForEach-Object { [char](Get-Random -Min 33 -Max 126) })"`) do set WIZ_PW=%%i
        for /f "usebackq delims=" %%i in (`powershell -NoProfile -Command "-join((1..48) | ForEach-Object { '{0:x2}' -f (Get-Random -Max 256) })"`) do set GW_TOK=%%i

        :: Replace placeholders in .env using PowerShell
        powershell -NoProfile -Command ^
            "(Get-Content .env) -replace 'REDIS_PASSWORD=CHANGE_ME_generate_a_random_password', ('REDIS_PASSWORD=' + $env:REDIS_PW) -replace 'POSTGRES_PASSWORD=CHANGE_ME_generate_a_random_password', ('POSTGRES_PASSWORD=' + $env:PG_PW) -replace 'AUTH_SECRET=CHANGE_ME_run_openssl_rand_base64_48', ('AUTH_SECRET=' + $env:AUTH_SEC) -replace 'OPENCLAW_GATEWAY_TOKEN=CHANGE_ME_run_openssl_rand_hex_24', ('OPENCLAW_GATEWAY_TOKEN=' + $env:GW_TOK) -replace '# WIZARD_ADMIN_PASSWORD=', ('WIZARD_ADMIN_PASSWORD=' + $env:WIZ_PW) | Set-Content .env"

        echo [OK] Generated .env with secure defaults
    ) else (
        echo [OK] .env already exists
    )
)

:: ── Launch ──────────────────────────────────────────────────

if "%MODE%"=="demo" (
    echo Starting demo mode...
    docker compose -f docker-compose.demo.yml up -d --build
) else (
    echo Building and starting services (first run takes ~90s^)...
    docker compose up -d --build
)

if errorlevel 1 (
    echo.
    echo [ERROR] Docker Compose failed. Check the output above.
    pause
    exit /b 1
)

:: ── Wait for healthchecks ───────────────────────────────────

echo Waiting for services to become healthy...
timeout /t 30 /nobreak >nul

:: ── Session logging setup ───────────────────────────────────
:: Create logs directory if it doesn't exist
if not exist logs mkdir logs

:: Generate timestamped session log filename
for /f "tokens=1-3 delims=/ " %%a in ("%date%") do set DSTAMP=%%c-%%a-%%b
for /f "tokens=1-3 delims=:." %%a in ("%time: =0%") do set TSTAMP=%%a%%b%%c
set SESSION_LOG=logs\session-%DSTAMP%-%TSTAMP%.txt

:: Write session header
echo === SAFEPAW SESSION START === > "%SESSION_LOG%"
echo Date: %date% %time% >> "%SESSION_LOG%"
echo Mode: %MODE% >> "%SESSION_LOG%"
echo ================================ >> "%SESSION_LOG%"
echo. >> "%SESSION_LOG%"

:: ── Start log aggregation (background) ──────────────────────
:: Pipes all container logs to the session file in real time.
:: Runs in a hidden window; stop.bat kills it by title.
start "SafePaw — Log Aggregator" /min cmd /c "docker compose logs -f --timestamps >> \"%~dp0%SESSION_LOG%\" 2>&1"

echo [OK] Session log: %SESSION_LOG%

:: ── Open Window 1: Process Monitor ──────────────────────────
:: Refreshes container status every 5 seconds in a dedicated window.
start "SafePaw — Process Monitor" cmd /c "powershell -NoProfile -Command \"while ($true) { Clear-Host; Write-Host '  SafePaw — Process Monitor' -ForegroundColor Cyan; Write-Host '  =========================' -ForegroundColor Cyan; Write-Host ''; docker ps -a --filter 'name=safepaw-' --format 'table {{.Names}}\t{{.State}}\t{{.Status}}\t{{.Ports}}' 2>$null; Write-Host ''; Write-Host '  Resources:' -ForegroundColor Yellow; docker stats --no-stream --filter 'name=safepaw-' --format '  {{.Name}}: CPU {{.CPUPerc}} / Mem {{.MemUsage}}' 2>$null; Write-Host ''; Write-Host '  (Refreshes every 5s | Close with stop.bat)' -ForegroundColor DarkGray; Start-Sleep 5 }\""

echo [OK] Process Monitor window opened

:: ── Open Window 2: Live Log Viewer ──────────────────────────
:: Tails the session log file in real time.
start "SafePaw — Live Logs" cmd /c "powershell -NoProfile -Command \"Write-Host '  SafePaw — Live Log Viewer' -ForegroundColor Green; Write-Host '  Tailing: %SESSION_LOG%' -ForegroundColor Green; Write-Host '  (Close with stop.bat)' -ForegroundColor DarkGray; Write-Host ''; Get-Content -Path '%~dp0%SESSION_LOG%' -Wait -Tail 200\""

echo [OK] Live Log Viewer window opened

:: ── Print summary ───────────────────────────────────────────

echo.
echo ==========================================
if "%MODE%"=="demo" (
    echo   Wizard:   http://localhost:3000
    echo   Password: DemoPassword123!
) else (
    echo   Wizard:   http://localhost:3000
    for /f "tokens=1,* delims==" %%a in ('findstr "^WIZARD_ADMIN_PASSWORD=" .env 2^>nul') do echo   Password: %%b
)
echo   Gateway:  http://localhost:8080
echo   Session:  %SESSION_LOG%
echo ==========================================
echo.

:: ── Open browser ────────────────────────────────────────────

echo Opening browser...
start http://localhost:3000

echo.
echo To stop:  stop.bat  (or start.bat --stop)
echo.
pause
