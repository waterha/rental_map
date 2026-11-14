param([int]$Port=8081)
$ErrorActionPreference = 'Stop'
$projectDirectory = Split-Path -Parent $PSScriptRoot
$previousAddress = $env:ADDR
$previousData = $env:DATA_DIR
$previousBase = $env:TEST_BASE_URL
$previousPublish = $env:ALLOW_PUBLISH
$previousDatabase = $env:DATABASE_URL
$previousSmtpDisabled = $env:SMTP_DISABLED
$testProcess = $null
Push-Location -LiteralPath $projectDirectory
try {
  $testDirectory = Join-Path $projectDirectory ('.local/e2e-' + [guid]::NewGuid().ToString('N'))
  New-Item -ItemType Directory -Path $testDirectory -Force | Out-Null
  Copy-Item -LiteralPath 'data/regions.json' -Destination (Join-Path $testDirectory 'regions.json')
  Copy-Item -LiteralPath 'data/maps' -Destination (Join-Path $testDirectory 'maps') -Recurse
  Copy-Item -LiteralPath 'data/images' -Destination (Join-Path $testDirectory 'images') -Recurse
  Copy-Item -LiteralPath 'data/metro' -Destination (Join-Path $testDirectory 'metro') -Recurse
  & npm.cmd run build
  if ($LASTEXITCODE -ne 0) { throw 'Frontend build failed.' }
  $testBinary = Join-Path $testDirectory 'test-server.exe'
  & go build -o $testBinary ./cmd/server
  if ($LASTEXITCODE -ne 0) { throw 'Go build failed.' }
  $env:ADDR = "127.0.0.1:$Port"
  $env:DATA_DIR = $testDirectory
  $env:TEST_BASE_URL = "http://127.0.0.1:$Port"
  $env:ALLOW_PUBLISH = 'true'
  $env:DATABASE_URL = ''
  # The isolated browser suite must never send real emails or use the local SMTP credential.
  $env:SMTP_DISABLED = 'true'
  $testProcess = Start-Process -FilePath $testBinary -WorkingDirectory $projectDirectory -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $testDirectory 'server.log') -RedirectStandardError (Join-Path $testDirectory 'server-error.log')
  $ready = $false
  for ($attempt=0; $attempt -lt 30; $attempt++) {
    if ($testProcess.HasExited) { throw 'Test server exited. Check its server-error.log.' }
    try {
      $health = Invoke-RestMethod -Uri "$($env:TEST_BASE_URL)/api/health" -TimeoutSec 1
      if ($health.status -eq 'ok') { $ready = $true; break }
    } catch { Start-Sleep -Milliseconds 200 }
  }
  if (-not $ready) { throw 'Test server did not become ready.' }
  & npm.cmd run test:e2e
  if ($LASTEXITCODE -ne 0) { throw 'Browser verification failed.' }
} finally {
  if ($null -ne $testProcess -and -not $testProcess.HasExited) { Stop-Process -Id $testProcess.Id }
  $env:ADDR = $previousAddress
  $env:DATA_DIR = $previousData
  $env:TEST_BASE_URL = $previousBase
  $env:ALLOW_PUBLISH = $previousPublish
  $env:DATABASE_URL = $previousDatabase
  $env:SMTP_DISABLED = $previousSmtpDisabled
  Pop-Location
}
