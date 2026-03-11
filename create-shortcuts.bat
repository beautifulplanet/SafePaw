@echo off
title SafePaw — Create Desktop Shortcuts
cd /d "%~dp0"

echo.
echo   Creating SafePaw desktop shortcuts...
echo.

set DESKTOP=%USERPROFILE%\Desktop
set SCRIPTDIR=%~dp0

:: ── SafePaw START shortcut ──────────────────────────────────
powershell -NoProfile -Command ^
  "$ws = New-Object -ComObject WScript.Shell; $s = $ws.CreateShortcut('%DESKTOP%\SafePaw START.lnk'); $s.TargetPath = '%SCRIPTDIR%start.bat'; $s.WorkingDirectory = '%SCRIPTDIR%'; $s.Description = 'Start SafePaw stack'; $s.Save()"
echo [OK] SafePaw START.lnk

:: ── SafePaw STOP shortcut ───────────────────────────────────
powershell -NoProfile -Command ^
  "$ws = New-Object -ComObject WScript.Shell; $s = $ws.CreateShortcut('%DESKTOP%\SafePaw STOP.lnk'); $s.TargetPath = '%SCRIPTDIR%stop.bat'; $s.WorkingDirectory = '%SCRIPTDIR%'; $s.Description = 'Stop SafePaw stack'; $s.Save()"
echo [OK] SafePaw STOP.lnk

:: ── SafePaw STATUS shortcut ─────────────────────────────────
powershell -NoProfile -Command ^
  "$ws = New-Object -ComObject WScript.Shell; $s = $ws.CreateShortcut('%DESKTOP%\SafePaw STATUS.lnk'); $s.TargetPath = '%SCRIPTDIR%status.bat'; $s.WorkingDirectory = '%SCRIPTDIR%'; $s.Description = 'Check SafePaw service status'; $s.Save()"
echo [OK] SafePaw STATUS.lnk

:: ── SafePaw DEMO shortcut ───────────────────────────────────
powershell -NoProfile -Command ^
  "$ws = New-Object -ComObject WScript.Shell; $s = $ws.CreateShortcut('%DESKTOP%\SafePaw DEMO.lnk'); $s.TargetPath = '%SCRIPTDIR%START-DEMO.bat'; $s.WorkingDirectory = '%SCRIPTDIR%'; $s.Description = 'Start SafePaw demo mode'; $s.Save()"
echo [OK] SafePaw DEMO.lnk

echo.
echo ==========================================
echo   Desktop shortcuts created:
echo     SafePaw START.lnk
echo     SafePaw STOP.lnk
echo     SafePaw STATUS.lnk
echo     SafePaw DEMO.lnk
echo ==========================================
echo.
pause
