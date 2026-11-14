param([switch]$SkipBuild)
$ErrorActionPreference = 'Stop'
$projectDirectory = Split-Path -Parent $PSScriptRoot
Push-Location -LiteralPath $projectDirectory
try {
  if (-not (Test-Path -LiteralPath 'data/regions.json')) {
    & node scripts/download-data.mjs
    if ($LASTEXITCODE -ne 0) { throw 'Map data download failed.' }
  }
  if (-not $SkipBuild) {
    if (-not (Test-Path -LiteralPath 'node_modules')) {
      & npm.cmd ci
      if ($LASTEXITCODE -ne 0) { throw 'Dependency installation failed.' }
    }
    & npm.cmd run build
    if ($LASTEXITCODE -ne 0) { throw 'Frontend build failed.' }
  }
  New-Item -ItemType Directory -Path '.local' -Force | Out-Null
  & go build -o .local/rental-atlas.exe ./cmd/server
  if ($LASTEXITCODE -ne 0) { throw 'Go build failed.' }
  Write-Host 'Rental Atlas: http://127.0.0.1:8080' -ForegroundColor Cyan
  & ./.local/rental-atlas.exe
} finally { Pop-Location }
