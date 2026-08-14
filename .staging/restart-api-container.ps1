$ErrorActionPreference = 'Continue'
docker stop starloader-api 2>&1 | Out-Null
for ($i = 0; $i -lt 15; $i++) {
    docker rm starloader-api 2>&1 | Out-Null
    $existing = docker ps -a --format '{{.Names}}' | Select-String '^starloader-api$'
    if (-not $existing) { break }
    Start-Sleep -Seconds 2
}
$existing = docker ps -a --format '{{.Names}}' | Select-String '^starloader-api$'
if ($existing) { throw 'old container still present' }

docker run -d --name starloader-api `
    -p 127.0.0.1:8080:8080 `
    -v 'C:\Users\pc\Desktop\Projelerim\StarLoader\backend:/workspace' `
    -w /workspace `
    -e 'DATABASE_URL=postgres://postgres:postgres@host.docker.internal:55432/starloader?sslmode=disable' `
    -e 'LICENSE_HMAC_KEY=BIRINCI_UZUN_RASTGELE_ANAHTAR' `
    -e 'HARDWARE_HMAC_KEY=IKINCI_FARKLI_UZUN_RASTGELE_ANAHTAR' `
    -e 'ED25519_PRIVATE_KEY=ODgtleiTIdhBilNHXxwEAFVPsr2kUa36FEtOVuUx6h0=' `
    -e 'LICENSE_ISSUER=starloader' `
    -e 'LICENSE_AUDIENCE=starloader-client' `
    -e 'PRODUCT=StarLoader' `
    -e 'SERVER_ADDR=:8080' `
    -e 'ADMIN_SESSION_SECRET=a580390360a63b65029411b169765ad7be6911e35a69187f232dd80d16d7ce74' `
    -e 'ADMIN_CONSOLE_ENABLED=true' `
    golang:1.24 go run ./cmd/server serve
if ($LASTEXITCODE -ne 0) { throw 'docker run failed' }
Write-Host 'container recreated, waiting for server...'
$ready = $false
for ($i = 0; $i -lt 60; $i++) {
    Start-Sleep -Seconds 3
    try {
        $h = Invoke-WebRequest -Uri 'http://127.0.0.1:8080/healthz' -UseBasicParsing -TimeoutSec 3
        if ($h.StatusCode -eq 200) { $ready = $true; break }
    } catch { }
}
if ($ready) { Write-Host 'server ready' } else { Write-Host 'server NOT ready yet' }
docker logs --tail 5 starloader-api
