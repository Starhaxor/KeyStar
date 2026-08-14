$session = New-Object Microsoft.PowerShell.Commands.WebRequestSession
try {
    $response = Invoke-WebRequest -Method POST -Uri "http://127.0.0.1:8080/v1/admin/auth/login" -ContentType "application/json" -Body '{"email":"admin@keystar.local","password":"YeniGucluAdmin2026!"}' -WebSession $session -UseBasicParsing
    Write-Host "STATUS: $($response.StatusCode)"
    Write-Host "BODY: $($response.Content)"
    Write-Host "COOKIES:"
    $session.Cookies.GetCookies("http://127.0.0.1:8080") | ForEach-Object { Write-Host "  $($_.Name)=$($_.Value)" }
} catch {
    Write-Host "EXCEPTION: $($_.Exception.Message)"
    Write-Host "ERROR DETAILS: $($_.ErrorDetails.Message)"
    if ($_.Exception.Response) { Write-Host "STATUS: $([int]$_.Exception.Response.StatusCode)" }
}
