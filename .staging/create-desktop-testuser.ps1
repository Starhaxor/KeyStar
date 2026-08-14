$ErrorActionPreference = 'Continue'
@('DesktopTest2026!', 'DesktopTest2026!') | docker exec -i starloader-api sh -c 'cd /workspace && go run ./cmd/server admin create-user --email desktest@example.com --password-stdin'
Write-Host "create-user exit: $LASTEXITCODE"
docker exec starloader-api sh -c 'cd /workspace && go run ./cmd/server admin create-license --user desktest@example.com --product StarLoader --days 30 --max-devices 2'
Write-Host "create-license exit: $LASTEXITCODE"
