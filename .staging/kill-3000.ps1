$conn = Get-NetTCPConnection -LocalPort 3000 -State Listen -ErrorAction SilentlyContinue | Select-Object -First 1
if ($conn) {
    Stop-Process -Id $conn.OwningProcess -Force
    Write-Host "killed $($conn.OwningProcess)"
    Start-Sleep -Seconds 2
} else {
    Write-Host "port 3000 free"
}
