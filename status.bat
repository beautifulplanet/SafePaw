@echo off
title SafePaw -- Status Check
cd /d "%~dp0"

echo.
echo   SafePaw — Service Status
echo   ========================
echo.

:: ── Check Docker ────────────────────────────────────────────
docker info >nul 2>&1
if errorlevel 1 (
    echo [ERROR] Docker is not running.
    echo.
    echo   SafePaw: UNKNOWN (Docker not available)
    echo.
    pause
    exit /b 1
)

:: ── Check if any SafePaw containers exist ───────────────────
docker ps -a --filter "name=safepaw-" --format "{{.Names}}" 2>nul | findstr "safepaw-" >nul 2>&1
if errorlevel 1 (
    echo   SafePaw: NOT DEPLOYED
    echo   No SafePaw containers found.
    echo.
    echo   To start: run start.bat or START-DEMO.bat
    echo.
    pause
    exit /b 0
)

:: ── Per-service status ──────────────────────────────────────
echo   Container               State        Health       Uptime
echo   --------------------    ---------    ---------    ---------------

:: Loop through all safepaw containers and format output
for /f "tokens=*" %%i in ('docker ps -a --filter "name=safepaw-" --format "{{.Names}}|{{.State}}|{{.Status}}"') do (
    call :parseline "%%i"
)

echo.

:: ── Summary ─────────────────────────────────────────────────
set TOTAL=0
set RUNNING=0
set HEALTHY=0

for /f %%n in ('docker ps -a --filter "name=safepaw-" --format "{{.Names}}" ^| find /c /v ""') do set TOTAL=%%n
for /f %%n in ('docker ps --filter "name=safepaw-" --filter "status=running" --format "{{.Names}}" ^| find /c /v ""') do set RUNNING=%%n
for /f %%n in ('docker ps --filter "name=safepaw-" --filter "health=healthy" --format "{{.Names}}" ^| find /c /v ""') do set HEALTHY=%%n

echo   -----------------------------------------
if %RUNNING%==%TOTAL% (
    echo   SafePaw: RUNNING (%RUNNING%/%TOTAL% up, %HEALTHY% healthy^)
) else if %RUNNING%==0 (
    echo   SafePaw: STOPPED (%RUNNING%/%TOTAL% up^)
) else (
    echo   SafePaw: DEGRADED (%RUNNING%/%TOTAL% up, %HEALTHY% healthy^)
)
echo   -----------------------------------------

:: ── Port bindings ───────────────────────────────────────────
echo.
echo   Endpoints:
for /f "tokens=*" %%p in ('docker ps --filter "name=safepaw-" --format "  {{.Names}}: {{.Ports}}" 2^>nul') do echo %%p

:: ── Resource usage ──────────────────────────────────────────
echo.
echo   Resources:
docker stats --no-stream --filter "name=safepaw-" --format "  {{.Name}}: CPU {{.CPUPerc}} / Mem {{.MemUsage}}" 2>nul

:: ── Recent session log ──────────────────────────────────────
echo.
echo   Session logs:
for /f "tokens=*" %%f in ('dir /b /o-d logs\session-*.txt 2^>nul') do (
    echo   Latest: logs\%%f
    goto :logfound
)
echo   No session logs found in logs\
:logfound

echo.
pause
exit /b 0

:: ── Helper: parse docker ps output line ─────────────────────
:parseline
set "LINE=%~1"
:: Replace | with spaces for display
set "LINE=%LINE:|=    %"
echo   %LINE%
exit /b 0
