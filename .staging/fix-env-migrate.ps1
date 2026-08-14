$ErrorActionPreference = "Stop"
Set-Location "C:\Users\pc\Desktop\Projelerim\StarLoader\backend"
$content = Get-Content .env -Raw
$pattern = "DATABASE_URL=[^\r\n]*"
$replacement = "DATABASE_URL=postgres://postgres:postgres@127.0.0.1:55432/starloader?sslmode=disable"
$newContent = [regex]::Replace($content, $pattern, $replacement)
Set-Content -Path .env -Value $newContent -NoNewline
Get-Content .env | ForEach-Object {
    if ($_ -match '^\s*([^#=\s][^=]*)=(.*)$') {
        [Environment]::SetEnvironmentVariable($Matches[1].Trim(), $Matches[2].Trim())
    }
}
Write-Output ("DATABASE_URL=" + $env:DATABASE_URL)
go run ./cmd/server migrate up
