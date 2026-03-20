@echo off
title SafePaw LITE Demo
cd /d "%~dp0"

echo.
echo ========================================
echo   SafePaw LITE Demo
echo   3 services, minimal resources
echo ========================================
echo.

docker compose -f docker-compose.demo.yml up -d --build
if errorlevel 1 (
    echo.
    echo [ERROR] Docker failed.
    echo   1. Is Docker Desktop running?
    echo   2. Start it, wait for "Running", try again.
    echo.
    pause
    exit /b 1
)

echo.
echo [OK] All 3 services starting (Wizard + Gateway + MockBackend)
echo.
echo Waiting 30 seconds for health checks...
timeout /t 30 /nobreak >nul

echo.
echo Opening browser...
start http://localhost:3000

echo.
echo ========================================
echo   Wizard:   http://localhost:3000
echo   Password: DemoPassword123!
echo   Gateway:  http://localhost:8080/health
echo ========================================
echo.
echo To stop:  docker compose -f docker-compose.demo.yml down
echo.
pause
