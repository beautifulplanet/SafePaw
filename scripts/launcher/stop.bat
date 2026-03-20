@echo off
title SafePaw -- Shutdown
set "ROOT=%~dp0..\..\\"
cd /d "%ROOT%"

echo.
echo   SafePaw — Shutting down...
echo.

:: ── Check if anything is running ────────────────────────────
docker info >nul 2>&1
if errorlevel 1 (
    echo [ERROR] Docker is not running. Nothing to stop.
    pause
    exit /b 1
)

:: ── Stop log viewer windows ────────────────────────────────
:: Kill any PowerShell windows we started for monitoring
:: (identified by their window titles)
echo Closing viewer windows...
taskkill /FI "WINDOWTITLE eq SafePaw — Process Monitor" /F >nul 2>&1
taskkill /FI "WINDOWTITLE eq SafePaw — Live Logs" /F >nul 2>&1

:: ── Stop the log aggregation background job ────────────────
:: The log aggregator runs as a background docker compose logs process.
:: Kill it by window title (started by start.bat).
taskkill /FI "WINDOWTITLE eq SafePaw — Log Aggregator" /F >nul 2>&1

:: ── Determine which compose file is active ──────────────────
:: Check if demo mode containers are running
docker ps --filter "name=safepaw-" --format "{{.Names}}" 2>nul | findstr "safepaw-" >nul 2>&1
if errorlevel 1 (
    echo [INFO] No SafePaw containers found running.
    echo.
    pause
    exit /b 0
)

:: ── Show what we're stopping ────────────────────────────────
echo Currently running SafePaw containers:
echo.
docker ps --filter "name=safepaw-" --format "  {{.Names}}  ({{.Status}})"
echo.

:: ── Graceful shutdown (full compose) ────────────────────────
echo Stopping full-mode services...
docker compose down --timeout 30 2>nul

:: ── Graceful shutdown (demo compose) ────────────────────────
echo Stopping demo-mode services (if any)...
docker compose -f docker-compose.demo.yml down --timeout 30 2>nul

:: ── Verify nothing remains ──────────────────────────────────
timeout /t 3 /nobreak >nul
docker ps --filter "name=safepaw-" --format "{{.Names}}" 2>nul | findstr "safepaw-" >nul 2>&1
if not errorlevel 1 (
    echo.
    echo [WARN] Some containers still running. Force-killing...
    for /f "tokens=*" %%c in ('docker ps --filter "name=safepaw-" --format "{{.Names}}"') do (
        echo   Force-stopping %%c
        docker kill %%c >nul 2>&1
        docker rm %%c >nul 2>&1
    )
)

:: ── Final status ────────────────────────────────────────────
echo.
echo ==========================================
echo   SafePaw: STOPPED
echo   All services shut down.
echo   Log files preserved in: logs\
echo ==========================================
echo.

:: ── Append shutdown event to session log ────────────────────
:: Find the most recent session log and append shutdown marker
for /f "tokens=*" %%f in ('dir /b /o-d logs\session-*.txt 2^>nul') do (
    echo [%date% %time%] === SAFEPAW SHUTDOWN (stop.bat) === >> "logs\%%f"
    goto :logged
)
:logged

pause
