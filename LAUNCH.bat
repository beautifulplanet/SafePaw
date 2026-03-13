@echo off
title SafePaw - Start / Stop
color 0A
set "ROOT=%~dp0"
cd /d "%ROOT%"
if not exist "%ROOT%start.bat" (
    echo ERROR: start.bat not found. Run LAUNCH.bat from the SafePaw folder.
    pause
    exit /b 1
)
for /f "delims=" %%i in ('powershell -NoProfile -ExecutionPolicy Bypass -Command "Get-Date -Format 'yyyy-MM-dd'" 2^>nul') do set TODAY=%%i
if not defined TODAY set TODAY=date-unknown

:menu
set STACK_STATUS=STOPPED
docker info >nul 2>&1
if errorlevel 1 (set STACK_STATUS=Docker not available) else (
  for /f %%i in ('docker ps --filter "name=safepaw" --format "{{.Names}}" 2^>nul') do set STACK_STATUS=RUNNING
)
cls
echo.
echo  ============================================================
echo    SAFEPAW - LAUNCHER
echo    Stack: %STACK_STATUS%
echo    SafePaw + OpenClaw or Demo. One click.
echo  ============================================================
echo.
echo  Start:
echo    [1]  Full stack (SafePaw + OpenClaw)
echo    [2]  Demo (SafePaw only, no API key)
echo.
echo  Stop:
echo    [3]  Shut down all
echo.
echo  Status:
echo    [4]  Show processes (SafePaw, OpenClaw, support)
echo    [5]  Quick health check (wizard + gateway)
echo.
echo  Logs: logs\%TODAY%\full^|demo^|shutdown^|processes\ (part1, part2...)
echo  ============================================================
echo.
set /p pick="Enter [1-5] or [Q] to quit: "

if /i "%pick%"=="Q" exit /b 0
if "%pick%"=="1"  goto run_full
if "%pick%"=="2"  goto run_demo
if "%pick%"=="3"  goto shutdown
if "%pick%"=="4"  goto show_processes
if "%pick%"=="5"  goto health_check
if "%pick%"==""   goto menu

echo  Invalid choice. Try 1, 2, 3, 4, 5, or Q.
timeout /t 2 >nul
goto menu

:run_full
docker ps --filter "name=safepaw" -q 2>nul | findstr . >nul 2>&1
if not errorlevel 1 (
    echo.
    echo  Stack is already running. Use [3] to stop first, or [4] to see processes.
    echo  Press any key to return to menu...
    pause >nul
    goto menu
)
:: Pre-check: OpenClaw build context must exist (default ../../openclaw or OPENCLAW_BUILD_CONTEXT in .env)
setlocal enabledelayedexpansion
set "OC_CTX="
if exist "%ROOT%.env" (
    for /f "usebackq tokens=2 delims==" %%a in ('findstr /b "OPENCLAW_BUILD_CONTEXT=" "%ROOT%.env" 2^>nul') do set "OC_CTX=%%~a"
)
if defined OC_CTX (set "OC_PATH=!OC_CTX!") else (set "OC_PATH=%ROOT%..\..\openclaw")
if not exist "!OC_PATH!\.\" (
    echo.
    echo  OpenClaw repo not found at: !OC_PATH!
    echo  Full stack needs the OpenClaw source. Either:
    echo    1. Clone OpenClaw into that folder, or
    echo    2. Set OPENCLAW_BUILD_CONTEXT in .env to your OpenClaw folder path.
    echo  Use [2] Demo for SafePaw-only (no OpenClaw).
    echo.
    echo  Press any key to return to menu...
    endlocal
    pause >nul
    goto menu
)
endlocal
:: Pre-check: ports 3000 and 8080 (SOW-002-C)
setlocal enabledelayedexpansion
set PORTS_USED=
netstat -an 2>nul | findstr ":3000 " >nul 2>&1
if not errorlevel 1 set PORTS_USED=1
netstat -an 2>nul | findstr ":8080 " >nul 2>&1
if not errorlevel 1 set PORTS_USED=1
if defined PORTS_USED (
    echo.
    echo  Port 3000 or 8080 is in use. Free the port or stop the other app, then try again.
    set /p PORT_ANYWAY="  Start anyway? [y/N]: "
    if /i not "!PORT_ANYWAY!"=="y" endlocal & goto menu
)
endlocal
:: Pre-check: .env placeholder secrets (CHANGE_ME)
if exist "%ROOT%.env" (
    findstr /C:"CHANGE_ME" "%ROOT%.env" >nul 2>&1
    if not errorlevel 1 (
        setlocal enabledelayedexpansion
        echo.
        echo  .env contains CHANGE_ME placeholders. Replace with real secrets before production.
        set /p ENV_ANYWAY="  Start anyway? [y/N]: "
        if /i not "!ENV_ANYWAY!"=="y" endlocal & goto menu
        endlocal
    )
)
powershell -NoProfile -ExecutionPolicy Bypass -File "%ROOT%Log-Launch.ps1" -Date "%TODAY%" -Action full -Message "Full stack start requested" >nul 2>&1
if errorlevel 1 (
    echo   Warning: launcher log could not be written. Check logs\launcher-errors.txt
    if not exist "%ROOT%logs" mkdir "%ROOT%logs"
    echo %date% %time% Log-Launch failed - full >> "%ROOT%logs\launcher-errors.txt"
)
echo.
echo  Starting full stack (SafePaw + OpenClaw)...
call "%ROOT%start.bat"
echo.
echo  Press any key to return to menu...
pause >nul
goto menu

:run_demo
docker ps --filter "name=safepaw" -q 2>nul | findstr . >nul 2>&1
if not errorlevel 1 (
    echo.
    echo  Stack is already running. Use [3] to stop first, or [4] to see processes.
    echo  Press any key to return to menu...
    pause >nul
    goto menu
)
:: Pre-check: ports 3000 and 8080 (SOW-002-C)
setlocal enabledelayedexpansion
set PORTS_USED=
netstat -an 2>nul | findstr ":3000 " >nul 2>&1
if not errorlevel 1 set PORTS_USED=1
netstat -an 2>nul | findstr ":8080 " >nul 2>&1
if not errorlevel 1 set PORTS_USED=1
if defined PORTS_USED (
    echo.
    echo  Port 3000 or 8080 is in use. Free the port or stop the other app, then try again.
    set /p PORT_ANYWAY="  Start anyway? [y/N]: "
    if /i not "!PORT_ANYWAY!"=="y" endlocal & goto menu
)
endlocal
:: Pre-check: .env placeholder secrets (CHANGE_ME)
if exist "%ROOT%.env" (
    findstr /C:"CHANGE_ME" "%ROOT%.env" >nul 2>&1
    if not errorlevel 1 (
        setlocal enabledelayedexpansion
        echo.
        echo  .env contains CHANGE_ME placeholders. Replace with real secrets before production.
        set /p ENV_ANYWAY="  Start anyway? [y/N]: "
        if /i not "!ENV_ANYWAY!"=="y" endlocal & goto menu
        endlocal
    )
)
powershell -NoProfile -ExecutionPolicy Bypass -File "%ROOT%Log-Launch.ps1" -Date "%TODAY%" -Action demo -Message "Demo start requested" >nul 2>&1
if errorlevel 1 (
    echo   Warning: launcher log could not be written. Check logs\launcher-errors.txt
    if not exist "%ROOT%logs" mkdir "%ROOT%logs"
    echo %date% %time% Log-Launch failed - demo >> "%ROOT%logs\launcher-errors.txt"
)
echo.
echo  Starting demo (SafePaw only)...
call "%ROOT%start.bat" --demo
echo.
echo  Press any key to return to menu...
pause >nul
goto menu

:shutdown
cd /d "%ROOT%"
docker ps --filter "name=safepaw" -q 2>nul | findstr . >nul 2>&1
if errorlevel 1 goto do_shutdown
echo.
set /p SHUTCONFIRM="Stack is running. Shut down all? [y/N]: "
if /i not "%SHUTCONFIRM%"=="y" goto menu
:do_shutdown
powershell -NoProfile -ExecutionPolicy Bypass -File "%ROOT%Log-Launch.ps1" -Date "%TODAY%" -Action shutdown -Message "Shut down requested" >nul 2>&1
if errorlevel 1 (
    echo   Warning: launcher log could not be written. Check logs\launcher-errors.txt
    if not exist "%ROOT%logs" mkdir "%ROOT%logs"
    echo %date% %time% Log-Launch failed - shutdown >> "%ROOT%logs\launcher-errors.txt"
)
echo.
echo  Shutting down all services...
docker compose down 2>nul
docker compose -f docker-compose.demo.yml down 2>nul
echo  [OK] All stopped.
powershell -NoProfile -ExecutionPolicy Bypass -File "%ROOT%Log-Launch.ps1" -Date "%TODAY%" -Action shutdown -Message "Shut down completed" >nul 2>&1
if errorlevel 1 (
    echo   Warning: launcher log could not be written. Check logs\launcher-errors.txt
    if not exist "%ROOT%logs" mkdir "%ROOT%logs"
    echo %date% %time% Log-Launch failed - shutdown-completed >> "%ROOT%logs\launcher-errors.txt"
)
echo.
echo  Press any key to return to menu...
pause >nul
goto menu

:health_check
cd /d "%ROOT%"
echo.
echo  Quick health check (localhost)...
for /f "delims=" %%a in ('powershell -NoProfile -Command "try { (Invoke-WebRequest -Uri 'http://127.0.0.1:3000/api/v1/health' -UseBasicParsing -TimeoutSec 3).StatusCode } catch { 'unreachable' }" 2^>nul') do set WIZ=%%a
for /f "delims=" %%a in ('powershell -NoProfile -Command "try { (Invoke-WebRequest -Uri 'http://127.0.0.1:8080/health' -UseBasicParsing -TimeoutSec 3).StatusCode } catch { 'unreachable' }" 2^>nul') do set GW=%%a
echo    Wizard  :3000: %WIZ%
echo    Gateway :8080: %GW%
echo.
if "%WIZ%"=="200" if "%GW%"=="200" (echo  [OK] Both healthy.) else (echo  [--] One or both not ready.)
if not "%WIZ%"=="200" (echo  [--] One or both not ready.)
echo.
echo  Press any key to return to menu...
pause >nul
goto menu

:show_processes
cd /d "%ROOT%"
powershell -NoProfile -ExecutionPolicy Bypass -File "%ROOT%Log-Launch.ps1" -Date "%TODAY%" -Action processes -Message "Show processes" >nul 2>&1
if errorlevel 1 (
    echo   Warning: launcher log could not be written. Check logs\launcher-errors.txt
    if not exist "%ROOT%logs" mkdir "%ROOT%logs"
    echo %date% %time% Log-Launch failed - processes >> "%ROOT%logs\launcher-errors.txt"
)
echo.
echo  ============================================================
echo    SAFEPAW / OPENCLAW - RUNNING PROCESSES
echo  ============================================================
echo.
docker ps --filter "name=safepaw" -q 2>nul > "%TEMP%\safepaw_ps.txt"
findstr /r "." "%TEMP%\safepaw_ps.txt" >nul 2>&1
if errorlevel 1 (
    echo  No SafePaw or OpenClaw containers running.
    echo  Start the stack with [1] or [2] to see them here.
    echo.
) else (
    docker ps --filter "name=safepaw"
)
echo.
echo  --- Legend ---
echo    SafePaw:   wizard :3000, gateway :8080
echo    Backend:   openclaw or mockbackend (internal)
echo    Support:   redis, postgres, docker-socket-proxy
echo  ============================================================
echo  (Snapshot logged to logs\%TODAY%\processes\)
echo.
echo  Press any key to return to menu...
pause >nul
goto menu
