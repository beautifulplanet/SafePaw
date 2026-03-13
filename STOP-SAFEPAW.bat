@echo off
title SafePaw - Emergency Stop
color 0C
cd /d "%~dp0"

if not exist "%~dp0docker-compose.yml" (
    echo ERROR: SafePaw folder not found. Run this from the SafePaw repo or use a shortcut with "Start in" set to the SafePaw folder.
    pause
    exit /b 1
)

echo.
echo  Stopping all SafePaw containers (full + demo)...
docker compose down 2>nul
docker compose -f docker-compose.demo.yml down 2>nul
echo.
echo  [OK] All SafePaw services stopped.
echo.
pause
