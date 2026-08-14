try {
    $r = Invoke-WebRequest -Method POST -Uri 'http://127.0.0.1:8080/v1/admin/auth/login' -ContentType 'application/json' -Body '{"email":"a@b.c","password":"xxxxxxxxxx"}' -UseBasicParsing
    Write-Host "ADMIN STATUS: $($r.StatusCode)"
    Write-Host "ADMIN BODY: $($r.Content)"
} catch {
    Write-Host "ADMIN STATUS: $([int]$_.Exception.Response.StatusCode)"
    Write-Host "ADMIN BODY: $($_.ErrorDetails.Message)"
}
try {
    $h = Invoke-WebRequest -Uri 'http://127.0.0.1:8080/healthz' -UseBasicParsing
    Write-Host "HEALTH STATUS: $($h.StatusCode)"
} catch {
    Write-Host "HEALTH STATUS: $([int]$_.Exception.Response.StatusCode)"
}
