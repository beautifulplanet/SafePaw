# SafePaw launcher smoke test (Windows).
# Run from repo root: .\scripts\Test-Launcher.ps1
# Verifies LAUNCH.bat runs and exits 0 when given "Q" (quit). No Docker required.

$ErrorActionPreference = "Stop"
$Root = Split-Path $PSScriptRoot -Parent
if (-not (Test-Path (Join-Path $Root "LAUNCH.bat"))) {
    Write-Host "FAIL: LAUNCH.bat not found. Run from SafePaw repo root or scripts/."
    exit 1
}

Push-Location $Root
try {
    $psi = New-Object System.Diagnostics.ProcessStartInfo
    $psi.FileName = "cmd.exe"
    $psi.Arguments = "/c echo Q | LAUNCH.bat"
    $psi.WorkingDirectory = $Root
    $psi.UseShellExecute = $false
    $psi.RedirectStandardOutput = $true
    $psi.RedirectStandardError = $true
    $psi.CreateNoWindow = $true
    $p = [System.Diagnostics.Process]::Start($psi)
    $stdout = $p.StandardOutput.ReadToEnd()
    $stderr = $p.StandardError.ReadToEnd()
    $p.WaitForExit(25000)
    if ($p.ExitCode -ne 0) {
        Write-Host "FAIL: LAUNCH.bat exited with code $($p.ExitCode)."
        if ($stderr) { Write-Host $stderr }
        exit 1
    }
    if ($stdout -notmatch "SAFEPAW|Stack:|Enter") {
        Write-Host "FAIL: Launcher output missing expected menu text."
        exit 1
    }
    Write-Host "PASS: Launcher ran and exited 0."
    exit 0
} finally {
    Pop-Location
}
