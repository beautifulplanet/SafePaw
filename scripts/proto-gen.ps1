# =============================================================
# SafePaw Protobuf Compilation Script (Windows/PowerShell)
# =============================================================
# Compiles .proto files into Go and TypeScript bindings.
#
# Usage:
#   .\scripts\proto-gen.ps1
#
# Prerequisites:
#   - protoc (Protocol Buffer compiler)
#   - protoc-gen-go: go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
#   - ts-proto:      npm install -g ts-proto
# =============================================================

$ErrorActionPreference = "Stop"

$ProjectRoot = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
if (-not $ProjectRoot) {
    $ProjectRoot = (Get-Location).Path
}

# Handle being called from project root or scripts dir
if (Test-Path "$ProjectRoot\shared\proto\common.proto") {
    # Already at project root
} elseif (Test-Path "shared\proto\common.proto") {
    $ProjectRoot = (Get-Location).Path
} else {
    Write-Error "Cannot find proto files. Run from project root or scripts directory."
    exit 1
}

$ProtoDir = Join-Path $ProjectRoot "shared\proto"
$GenGoDir = Join-Path $ProjectRoot "shared\proto\gen\go"
$GenTsDir = Join-Path $ProjectRoot "shared\proto\gen\ts"

Write-Host "SafePaw Proto Compiler" -ForegroundColor Cyan
Write-Host "========================="

# Check prerequisites
$missing = @()
if (-not (Get-Command protoc -ErrorAction SilentlyContinue)) { $missing += "protoc" }
if (-not (Get-Command protoc-gen-go -ErrorAction SilentlyContinue)) { $missing += "protoc-gen-go" }
if (-not (Get-Command protoc-gen-ts_proto -ErrorAction SilentlyContinue)) { $missing += "protoc-gen-ts_proto (npm install -g ts-proto)" }

if ($missing.Count -gt 0) {
    Write-Error "Missing prerequisites: $($missing -join ', ')"
    exit 1
}

# Create output directories
New-Item -ItemType Directory -Force -Path $GenGoDir, $GenTsDir | Out-Null

# Clean previous generated files (keep go.mod/go.sum/package.json)
Get-ChildItem -Path $GenGoDir -Filter "*.pb.go" -ErrorAction SilentlyContinue | Remove-Item -Force
Get-ChildItem -Path $GenTsDir -Filter "*.ts" -ErrorAction SilentlyContinue | Remove-Item -Force

# Collect proto files
$protoFiles = Get-ChildItem -Path $ProtoDir -Filter "*.proto" | ForEach-Object { $_.FullName }
$protoCount = $protoFiles.Count
Write-Host "Found $protoCount .proto files"

# Generate Go bindings
Write-Host "  -> Go bindings..." -ForegroundColor Yellow
& protoc `
    --proto_path="$ProtoDir" `
    --go_out="$GenGoDir" `
    --go_opt=module=nopenclaw/proto `
    @protoFiles

if ($LASTEXITCODE -ne 0) { Write-Error "Go proto generation failed"; exit 1 }

# Generate TypeScript bindings
Write-Host "  -> TypeScript bindings..." -ForegroundColor Yellow
$tsPlugin = Join-Path (Split-Path (Get-Command protoc-gen-ts_proto).Source) "protoc-gen-ts_proto.cmd"
if (-not (Test-Path $tsPlugin)) {
    # Fallback: try the .ps1 wrapper's directory
    $tsPlugin = (Get-Command protoc-gen-ts_proto.cmd -ErrorAction SilentlyContinue).Source
}

& protoc `
    --proto_path="$ProtoDir" `
    --plugin="protoc-gen-ts_proto=$tsPlugin" `
    --ts_proto_out="$GenTsDir" `
    --ts_proto_opt=esModuleInterop=true `
    --ts_proto_opt=outputEncodeMethods=false `
    --ts_proto_opt=outputJsonMethods=true `
    @protoFiles

if ($LASTEXITCODE -ne 0) { Write-Error "TypeScript proto generation failed"; exit 1 }

# Verify Go compiles
Write-Host "  -> Verifying Go compilation..." -ForegroundColor Yellow
Push-Location $GenGoDir
try {
    if (-not (Test-Path "go.mod")) {
        & go mod init nopenclaw/proto 2>&1 | Out-Null
        & go mod tidy 2>&1 | Out-Null
    }
    & go build ./... 2>&1
    if ($LASTEXITCODE -ne 0) { Write-Error "Generated Go code does not compile"; exit 1 }
} finally {
    Pop-Location
}

# Summary
$goFiles = (Get-ChildItem -Path $GenGoDir -Filter "*.pb.go" -Recurse).Count
$tsFiles = (Get-ChildItem -Path $GenTsDir -Filter "*.ts" -Recurse).Count
Write-Host ""
Write-Host "Proto compilation complete!" -ForegroundColor Green
Write-Host "  Go:         $goFiles files in shared/proto/gen/go/"
Write-Host "  TypeScript: $tsFiles files in shared/proto/gen/ts/"
