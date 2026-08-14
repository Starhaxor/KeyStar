$ErrorActionPreference = 'Stop'
$staging = 'c:\Users\pc\Desktop\Projelerim\KeyStar\.staging\backend'
$target  = 'C:\Users\pc\Desktop\Projelerim\StarLoader\backend'

$files = @(
    'internal\config\config.go',
    'cmd\server\main.go'
)
foreach ($file in $files) {
    Copy-Item -Path (Join-Path $staging $file) -Destination (Join-Path $target $file) -Force
    Write-Host "copied $file"
}

Push-Location $target
try {
    gofmt -w internal/config/config.go cmd/server/main.go
    go build ./...
    if ($LASTEXITCODE -ne 0) { throw 'go build failed' }
    go vet ./...
    if ($LASTEXITCODE -ne 0) { throw 'go vet failed' }
    Write-Host 'BUILD + VET OK'
} finally {
    Pop-Location
}
