param([string]$AdminConsoleEnabled = '')
$ErrorActionPreference = 'Stop'
$backend = 'C:\Users\pc\Desktop\Projelerim\StarLoader\backend'

$conns = Get-NetTCPConnection -LocalPort 8080 -State Listen -ErrorAction SilentlyContinue
if ($conns) {
    $conns | ForEach-Object { Stop-Process -Id $_.OwningProcess -Force -ErrorAction SilentlyContinue }
    Start-Sleep -Seconds 2
}

Set-Location $backend
Get-Content (Join-Path $backend '.env') | ForEach-Object {
    $line = $_.Trim()
    if ($line -and -not $line.StartsWith('#') -and $line.Contains('=')) {
        $idx = $line.IndexOf('=')
        $name = $line.Substring(0, $idx).Trim()
        $value = $line.Substring($idx + 1).Trim().Trim('"')
        Set-Item -Path "Env:$name" -Value $value
    }
}
if ($AdminConsoleEnabled -ne '') {
    Set-Item -Path 'Env:ADMIN_CONSOLE_ENABLED' -Value $AdminConsoleEnabled
    Write-Host "override ADMIN_CONSOLE_ENABLED=$AdminConsoleEnabled"
}
& go run ./cmd/server
