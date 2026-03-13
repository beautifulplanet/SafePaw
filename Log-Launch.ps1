# SafePaw Launcher — structured logging with date folder, per-app subfolder, size-limited parts
# Usage: .\Log-Launch.ps1 -Date "2026-03-10" -Action "full" -Message "Full stack start requested"
#        .\Log-Launch.ps1 -Date "2026-03-10" -Action "processes" -Message "Show processes"
# Logs go to: logs\YYYY-MM-DD\{full|demo|shutdown|processes}\part1.log (part2.log when part1 exceeds MaxPartBytes)

param(
    [Parameter(Mandatory=$true)] [string]$Date,
    [Parameter(Mandatory=$true)] [ValidateSet("full","demo","shutdown","processes")] [string]$Action,
    [Parameter(Mandatory=$true)] [string]$Message
)

$ErrorActionPreference = "Stop"
$MaxPartBytes = 100 * 1024   # 100 KB per part file; then rotate to part2, part3, ...
$ScriptDir = $PSScriptRoot
$LogBase = Join-Path $ScriptDir "logs" $Date $Action

if (-not (Test-Path $LogBase)) {
    New-Item -ItemType Directory -Path $LogBase -Force | Out-Null
}

# Find current part file (highest partN.log that exists and is under size, or create next)
$PartIndex = 1
$PartPath = Join-Path $LogBase "part$PartIndex.log"
while (Test-Path $PartPath) {
    $Len = (Get-Item $PartPath).Length
    if ($Len -lt $MaxPartBytes) { break }
    $PartIndex++
    $PartPath = Join-Path $LogBase "part$PartIndex.log"
}

$Timestamp = Get-Date -Format "yyyy-MM-dd HH:mm:ss"
$Line = "$Timestamp  $Message"

Add-Content -Path $PartPath -Value $Line -Encoding UTF8

if ($Action -eq "processes") {
    Add-Content -Path $PartPath -Value "" -Encoding UTF8
    Add-Content -Path $PartPath -Value "--- SafePaw/OpenClaw processes at $Timestamp ---" -Encoding UTF8
    try {
        $DockerOut = docker ps --filter "name=safepaw" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}" 2>&1
        if ($DockerOut) { Add-Content -Path $PartPath -Value $DockerOut -Encoding UTF8 }
    } catch {
        Add-Content -Path $PartPath -Value "docker ps error: $_" -Encoding UTF8
    }
    Add-Content -Path $PartPath -Value "" -Encoding UTF8
}

# Return path for batch to show user
Write-Output $PartPath
