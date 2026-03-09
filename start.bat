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
echo ==========================================
echo.

:: ── Open browser ────────────────────────────────────────────

echo Opening browser...
start http://localhost:3000

echo.
echo To stop:  start.bat --stop
echo.
pause
