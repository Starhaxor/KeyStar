$ErrorActionPreference = "Stop"
$base = "http://127.0.0.1:8080/v1/admin"
$script:pass = 0; $script:fail = 0
function Check($name, $ok) {
    if ($ok) { $script:pass++; Write-Host "PASS: $name" } else { $script:fail++; Write-Host "FAIL: $name" }
}
function Get-TotpCodeAt([string]$base32Secret, [long]$unixSeconds) {
    $alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"
    $bits = ""
    foreach ($char in $base32Secret.ToUpper().ToCharArray()) {
        $index = $alphabet.IndexOf($char)
        if ($index -lt 0) { continue }
        $bits += [Convert]::ToString($index, 2).PadLeft(5, '0')
    }
    $bytes = New-Object System.Collections.Generic.List[byte]
    for ($i = 0; $i + 8 -le $bits.Length; $i += 8) {
        $bytes.Add([Convert]::ToByte($bits.Substring($i, 8), 2))
    }
    $counter = [long][math]::Floor($unixSeconds / 30)
    $counterBytes = New-Object byte[] 8
    for ($i = 7; $i -ge 0; $i--) { $counterBytes[$i] = [byte]($counter -band 0xFF); $counter = [long]($counter -shr 8) }
    $hmac = New-Object System.Security.Cryptography.HMACSHA1
    $hmac.Key = $bytes.ToArray()
    $hash = $hmac.ComputeHash($counterBytes)
    $offset = $hash[19] -band 0x0F
    $value = (([int]($hash[$offset] -band 0x7F)) -shl 24) -bor ([int]$hash[$offset + 1] -shl 16) -bor ([int]$hash[$offset + 2] -shl 8) -bor [int]$hash[$offset + 3]
    return ($value % 1000000).ToString("D6")
}
function Invoke-Json($method, $url, $session, $body, $csrf) {
    $params = @{ Method = $method; Uri = $url; WebSession = $session; UseBasicParsing = $true }
    if ($body) {
        $params["ContentType"] = "application/json"
        $params["Body"] = $body
    }
    if ($csrf) { $params["Headers"] = @{ "X-CSRF-Token" = $csrf } }
    try {
        $response = Invoke-WebRequest @params
        return @{ Status = $response.StatusCode; Body = ($response.Content | ConvertFrom-Json) }
    } catch {
        $raw = $_.ErrorDetails.Message
        $status = [int]$_.Exception.Response.StatusCode
        $parsed = $null
        if ($raw) { try { $parsed = $raw | ConvertFrom-Json } catch { } }
        return @{ Status = $status; Body = $parsed }
    }
}

$session = New-Object Microsoft.PowerShell.Commands.WebRequestSession

# 1. login + MFA completion (TOTP computed from DB secret)
$login = Invoke-Json "POST" "$base/auth/login" $session '{"email":"admin@keystar.local","password":"YeniGucluAdmin2026!"}'
Check "login ok, mfa required" ($login.Status -eq 200 -and $login.Body.ok -eq $true -and $login.Body.mfa_required -eq $true)
$totpSecret = (& docker exec deploy-postgres-1 psql -U postgres -d starloader -t -A -c "select totp_secret from admin_accounts where email='admin@keystar.local'").Trim()
$epoch = [long]([DateTimeOffset]::UtcNow.ToUnixTimeSeconds())
$code = Get-TotpCodeAt $totpSecret $epoch
$mfa = Invoke-Json "POST" "$base/auth/mfa" $session ('{"mfa_token":"' + $login.Body.mfa_token + '","code":"' + $code + '"}')
Check "mfa verify ok" ($mfa.Status -eq 200 -and $mfa.Body.ok -eq $true)
$csrf = ($session.Cookies.GetCookies("http://127.0.0.1:8080") | Where-Object { $_.Name -eq "starloader_admin_csrf" }).Value
Check "csrf cookie present" ($csrf -ne $null -and $csrf.Length -gt 0)

# 2. create user
$email = "gaptest" + (Get-Random -Maximum 99999) + "@example.com"
$created = Invoke-Json "POST" "$base/users" $session ('{"email":"' + $email + '","password":"DenemeSifre2026!"}') $csrf
Check "create user 200" ($created.Status -eq 200 -and $created.Body.ok -eq $true)
$newUserID = $created.Body.user.id
Check "create user returns id+email" ($newUserID -ne $null -and $created.Body.user.email -eq $email)

# 3. duplicate user rejected
$dup = Invoke-Json "POST" "$base/users" $session ('{"email":"' + $email + '","password":"DenemeSifre2026!"}') $csrf
Check "duplicate user 409 USER_ALREADY_EXISTS" ($dup.Status -eq 409 -and $dup.Body.code -eq "USER_ALREADY_EXISTS")

# 4. short password rejected
$short = Invoke-Json "POST" "$base/users" $session '{"email":"shortpw@example.com","password":"kisa"}' $csrf
Check "short user password 400" ($short.Status -eq 400)

# 5. revoke user sessions (no sessions yet -> revoked 0)
$rev = Invoke-Json "POST" "$base/users/$newUserID/sessions/revoke" $session "" $csrf
Check "revoke user sessions ok" ($rev.Status -eq 200 -and $rev.Body.ok -eq $true -and $rev.Body.revoked -eq 0)

# 6. revoke sessions for unknown user -> 404
$revMissing = Invoke-Json "POST" "$base/users/0198c0e8-0000-7000-8000-000000000000/sessions/revoke" $session "" $csrf
Check "revoke sessions unknown user 404" ($revMissing.Status -eq 404)

# 7. devices list exposes hwid presence fields
$devices = Invoke-Json "GET" "$base/devices" $session
Check "devices list 200" ($devices.Status -eq 200 -and $devices.Body.ok -eq $true)

# 8. device detail unknown -> 404
$devMissing = Invoke-Json "GET" "$base/devices/0198c0e8-0000-7000-8000-000000000000" $session
Check "device detail unknown 404" ($devMissing.Status -eq 404)

# 9. device reset unknown -> 404
$resetMissing = Invoke-Json "POST" "$base/devices/0198c0e8-0000-7000-8000-000000000000/reset" $session "" $csrf
Check "device reset unknown 404" ($resetMissing.Status -eq 404)

# 10. create admin
$adminEmail = "gapadmin" + (Get-Random -Maximum 99999) + "@example.com"
$adminCreated = Invoke-Json "POST" "$base/admins" $session ('{"email":"' + $adminEmail + '","password":"CokGucluAdmin2026!","role":"viewer"}') $csrf
Check "create admin 201" ($adminCreated.Status -eq 201 -and $adminCreated.Body.ok -eq $true)
Check "created admin role viewer, mfa not enrolled" ($adminCreated.Body.admin.role -eq "viewer" -and $adminCreated.Body.admin.mfa_enrolled -eq $false)

# 11. duplicate admin rejected
$adminDup = Invoke-Json "POST" "$base/admins" $session ('{"email":"' + $adminEmail + '","password":"CokGucluAdmin2026!","role":"viewer"}') $csrf
Check "duplicate admin 409 ADMIN_ALREADY_EXISTS" ($adminDup.Status -eq 409 -and $adminDup.Body.code -eq "ADMIN_ALREADY_EXISTS")

# 12. invalid role rejected
$adminBadRole = Invoke-Json "POST" "$base/admins" $session '{"email":"badrole@example.com","password":"CokGucluAdmin2026!","role":"superuser"}' $csrf
Check "invalid role 400 ROLE_NOT_FOUND" ($adminBadRole.Status -eq 400 -and $adminBadRole.Body.code -eq "ROLE_NOT_FOUND")

# 13. short admin password rejected
$adminShort = Invoke-Json "POST" "$base/admins" $session '{"email":"shortadmin@example.com","password":"kisa","role":"viewer"}' $csrf
Check "short admin password 400" ($adminShort.Status -eq 400)

# 14. audit log contains USER_CREATED + ADMIN_CREATED
$audit = Invoke-Json "GET" "$base/audit-logs?page_size=50" $session
$actions = @($audit.Body.items | ForEach-Object { $_.action })
Check "audit has USER_CREATED" ($actions -contains "USER_CREATED")
Check "audit has ADMIN_CREATED" ($actions -contains "ADMIN_CREATED")

Write-Host ""
Write-Host "RESULT: $($script:pass) passed, $($script:fail) failed"
exit $script:fail
