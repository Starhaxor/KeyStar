$ErrorActionPreference = 'Stop'
$staging = 'c:\Users\pc\Desktop\Projelerim\KeyStar\.staging\backend'
$target  = 'C:\Users\pc\Desktop\Projelerim\StarLoader\backend'

$files = @(
    'internal\config\config.go',
    'internal\config\config_test.go',
    'internal\httpapi\admin.go',
    'internal\httpapi\admin_test.go',
    'cmd\server\main.go'
)
foreach ($file in $files) {
    Copy-Item -Path (Join-Path $staging $file) -Destination (Join-Path $target $file) -Force
    Write-Host "copied $file"
}

Push-Location $target
try {
    go build ./...
    if ($LASTEXITCODE -ne 0) { throw 'go build failed' }
    go vet ./...
    if ($LASTEXITCODE -ne 0) { throw 'go vet failed' }
    $testOut = go test ./internal/config/ ./internal/httpapi/ 2>&1
    if ($LASTEXITCODE -ne 0) { $testOut | Select-Object -Last 15; throw 'go test failed' }
    $testOut | Select-Object -Last 5
    Write-Host 'BUILD + VET + TEST OK'
} finally {
    Pop-Location
}
