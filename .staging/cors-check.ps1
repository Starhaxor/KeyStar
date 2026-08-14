function Test-Origin([string]$origin) {
    Write-Host "=== ORIGIN: $origin ==="
    # preflight
    try {
        $o = Invoke-WebRequest -Method OPTIONS -Uri 'http://127.0.0.1:8080/v1/admin/auth/login' -Headers @{Origin=$origin; 'Access-Control-Request-Method'='POST'; 'Access-Control-Request-Headers'='content-type,x-csrf-token'} -UseBasicParsing
        Write-Host "PREFLIGHT STATUS: $($o.StatusCode)"
        Write-Host ("ACAO: " + $o.Headers['Access-Control-Allow-Origin'])
    } catch {
        Write-Host "PREFLIGHT FAIL: $($_.Exception.Message)"
        if ($_.Exception.Response) {
            Write-Host "  status: $([int]$_.Exception.Response.StatusCode)"
            Write-Host ("  ACAO: " + $_.Exception.Response.Headers['Access-Control-Allow-Origin'])
        }
    }
    # actual login post
    try {
        $r = Invoke-WebRequest -Method POST -Uri 'http://127.0.0.1:8080/v1/admin/auth/login' -ContentType 'application/json' -Headers @{Origin=$origin} -Body '{"email":"challenge@example.com","password":"whatever12345"}' -UseBasicParsing
        Write-Host "LOGIN STATUS: $($r.StatusCode)"
        Write-Host ("ACAO: " + $r.Headers['Access-Control-Allow-Origin'])
    } catch {
        if ($_.Exception.Response) {
            Write-Host "LOGIN STATUS: $([int]$_.Exception.Response.StatusCode)"
            Write-Host ("ACAO: " + $_.Exception.Response.Headers['Access-Control-Allow-Origin'])
            Write-Host "BODY: $($_.ErrorDetails.Message)"
        } else {
            Write-Host "LOGIN FAIL: $($_.Exception.Message)"
        }
    }
}
Test-Origin 'http://localhost:3000'
Test-Origin 'http://127.0.0.1:3000'
