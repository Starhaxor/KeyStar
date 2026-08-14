$ErrorActionPreference = "Stop"
Set-Location "C:\Users\pc\Desktop\Projelerim\StarLoader\backend"
Get-Content .env | ForEach-Object {
    if ($_ -match '^\s*([^#=\s][^=]*)=(.*)$') {
        [Environment]::SetEnvironmentVariable($Matches[1].Trim(), $Matches[2].Trim())
    }
}
$password = "YeniGucluAdmin2026!"
$pipeText = "{0}`n{0}" -f $password
$pipeText | go run ./cmd/server admin create-admin --email admin@keystar.local --role owner --password-stdin
if ($LASTEXITCODE -ne 0) { throw "create-admin failed" }
Write-Output "ADMIN CREATED"
