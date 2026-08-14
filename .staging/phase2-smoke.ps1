$ErrorActionPreference = "Stop"
$base = "http://127.0.0.1:8080/v1/admin"
$email = "admin@keystar.local"
$password = "YeniGucluAdmin2026!"
$passed = 0
$failed = 0

function Check($name, $condition, $detail) {
    if ($condition) { Write-Output "PASS: $name"; $script:passed++ }
    else { Write-Output "FAIL: $name -- $detail"; $script:failed++ }
}

function Get-TotpCode([string]$base32Secret) {
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
    $counter = [long][math]::Floor(([DateTimeOffset]::UtcNow.ToUnixTimeSeconds()) / 30)
    $counterBytes = New-Object byte[] 8
    for ($i = 7; $i -ge 0; $i--) { $counterBytes[$i] = [byte]($counter -band 0xFF); $counter = [long]($counter -shr 8) }
    $hmac = New-Object System.Security.Cryptography.HMACSHA1
    $hmac.Key = $bytes.ToArray()
    $hash = $hmac.ComputeHash($counterBytes)
    $offset = $hash[19] -band 0x0F
    $value = (([int]($hash[$offset] -band 0x7F)) -shl 24) -bor ([int]$hash[$offset + 1] -shl 16) -bor ([int]$hash[$offset + 2] -shl 8) -bor [int]$hash[$offset + 3]
    return ($value % 1000000).ToString("D6")
}

function Wait-FreshTotpWindow {
    $remaining = 30 - ([DateTimeOffset]::UtcNow.ToUnixTimeSeconds() % 30)
    if ($remaining -lt 4) { Start-Sleep -Seconds ($remaining + 1) }
}

function Get-CookieValue($session, $name) {
    foreach ($cookie in $session.Cookies.GetCookies("http://127.0.0.1:8080")) {
        if ($cookie.Name -eq $name) { return $cookie.Value }
    }
    return ""
}

# 1. Login without MFA enrolled -> direct session
$response = Invoke-WebRequest -UseBasicParsing -Method POST -Uri "$base/auth/login" -ContentType "application/json" -Body (@{email=$email; password=$password} | ConvertTo-Json) -SessionVariable adminSession
$loginBody = $response.Content | ConvertFrom-Json
Check "login returns ok" ($loginBody.ok -eq $true) $response.Content
Check "login sets session cookie" ((Get-CookieValue $adminSession "starloader_admin_session") -ne "") "no session cookie"
$csrf = Get-CookieValue $adminSession "starloader_admin_csrf"
$headers = @{ "X-CSRF-Token" = $csrf }

# 2. /me exposes role, permissions, mfa flag
$response = Invoke-WebRequest -UseBasicParsing -Method GET -Uri "$base/me" -WebSession $adminSession
$me = $response.Content | ConvertFrom-Json
Check "me returns role owner" ($me.role -eq "owner") $response.Content
Check "me returns permissions" ($me.permissions.Count -ge 13) $response.Content
Check "me reports mfa not enrolled" ($me.mfa_enrolled -eq $false) $response.Content

# 3. Enrollment gate blocks console routes
try {
    $response = Invoke-WebRequest -UseBasicParsing -Method GET -Uri "$base/overview" -WebSession $adminSession
    Check "overview gated before enrollment" $false ("status " + $response.StatusCode)
} catch {
    $status = [int]$_.Exception.Response.StatusCode
    $body = "$($_.ErrorDetails.Message)"
    Check "overview gated before enrollment" ($status -eq 403 -and $body -like "*MFA_ENROLLMENT_REQUIRED*") ("status=$status body=$body")
}

# 4. Start TOTP enrollment
$response = Invoke-WebRequest -UseBasicParsing -Method POST -Uri "$base/mfa/enroll/start" -WebSession $adminSession -Headers $headers
$enroll = $response.Content | ConvertFrom-Json
Check "enroll start returns secret" ($enroll.secret.Length -ge 16) $response.Content
Check "enroll start returns otpauth uri" ($enroll.provisioning_uri -like "otpauth://totp/*") $response.Content

# 5. Confirm enrollment with a computed TOTP code
Wait-FreshTotpWindow
$code = Get-TotpCode $enroll.secret
$response = Invoke-WebRequest -UseBasicParsing -Method POST -Uri "$base/mfa/enroll/confirm" -WebSession $adminSession -Headers $headers -ContentType "application/json" -Body (@{code=$code} | ConvertTo-Json)
$confirm = $response.Content | ConvertFrom-Json
Check "enroll confirm returns 10 recovery codes" ($confirm.recovery_codes.Count -eq 10) $response.Content
$recoveryCode = $confirm.recovery_codes[0]

# 6. Console unlocked after enrollment
$response = Invoke-WebRequest -UseBasicParsing -Method GET -Uri "$base/overview" -WebSession $adminSession
Check "overview works after enrollment" ($response.StatusCode -eq 200) $response.Content

# 7. Logout, login again -> MFA challenge
Invoke-WebRequest -UseBasicParsing -Method POST -Uri "$base/auth/logout" -WebSession $adminSession -Headers $headers | Out-Null
$response = Invoke-WebRequest -UseBasicParsing -Method POST -Uri "$base/auth/login" -ContentType "application/json" -Body (@{email=$email; password=$password} | ConvertTo-Json) -SessionVariable mfaSession
$secondLogin = $response.Content | ConvertFrom-Json
Check "second login demands mfa" ($secondLogin.mfa_required -eq $true) $response.Content
Check "second login returns mfa token" ($secondLogin.mfa_token.Length -gt 20) $response.Content
Check "second login sets no session cookie" ((Get-CookieValue $mfaSession "starloader_admin_session") -eq "") "session cookie present"

# 8. Wrong MFA code rejected
try {
    $response = Invoke-WebRequest -UseBasicParsing -Method POST -Uri "$base/auth/mfa" -ContentType "application/json" -Body (@{mfa_token=$secondLogin.mfa_token; code="000000"} | ConvertTo-Json)
    Check "wrong mfa code rejected" $false ("status " + $response.StatusCode)
} catch {
    $status = [int]$_.Exception.Response.StatusCode
    Check "wrong mfa code rejected" ($status -eq 401) "status=$status"
}

# 9. Fresh challenge + valid TOTP completes login (re-login because challenge was consumed)
$response = Invoke-WebRequest -UseBasicParsing -Method POST -Uri "$base/auth/login" -ContentType "application/json" -Body (@{email=$email; password=$password} | ConvertTo-Json) -SessionVariable mfaSession
$challenge = ($response.Content | ConvertFrom-Json).mfa_token
Wait-FreshTotpWindow
$secretHolder = $enroll.secret
$code = Get-TotpCode $secretHolder
$response = Invoke-WebRequest -UseBasicParsing -Method POST -Uri "$base/auth/mfa" -ContentType "application/json" -Body (@{mfa_token=$challenge; code=$code} | ConvertTo-Json) -SessionVariable mfaSession
Check "mfa login completes with totp" ($response.StatusCode -eq 200) $response.Content
Check "mfa login sets session cookie" ((Get-CookieValue $mfaSession "starloader_admin_session") -ne "") "no session cookie"
$csrf = Get-CookieValue $mfaSession "starloader_admin_csrf"
$headers = @{ "X-CSRF-Token" = $csrf }

# 10. New phase-2 endpoints
$response = Invoke-WebRequest -UseBasicParsing -Method GET -Uri "$base/admins" -WebSession $mfaSession
$admins = $response.Content | ConvertFrom-Json
Check "admins list works" ($admins.ok -eq $true -and $admins.total -ge 1) $response.Content
$response = Invoke-WebRequest -UseBasicParsing -Method GET -Uri "$base/roles" -WebSession $mfaSession
$roles = $response.Content | ConvertFrom-Json
$roleNames = @($roles.items | ForEach-Object { $_.name })
Check "roles list has owner and viewer" (($roleNames -contains "owner") -and ($roleNames -contains "viewer")) $response.Content
$response = Invoke-WebRequest -UseBasicParsing -Method GET -Uri "$base/security-events" -WebSession $mfaSession
$events = $response.Content | ConvertFrom-Json
Check "security events list works" ($events.ok -eq $true -and $events.total -ge 1) $response.Content
$eventKinds = @($events.items | ForEach-Object { $_.kind })
Check "failed mfa attempt recorded" ($eventKinds -contains "ADMIN_MFA_FAILED") ($response.Content)

# 11. RBAC: create a viewer admin via CLI is tested separately; verify PATCH self guard
$ownId = (Invoke-WebRequest -UseBasicParsing -Method GET -Uri "$base/me" -WebSession $mfaSession | ConvertFrom-Json).id
try {
    $response = Invoke-WebRequest -UseBasicParsing -Method PATCH -Uri "$base/admins/$ownId" -WebSession $mfaSession -Headers $headers -ContentType "application/json" -Body '{"role":"viewer"}'
    Check "self modification rejected" $false ("status " + $response.StatusCode)
} catch {
    $status = [int]$_.Exception.Response.StatusCode
    Check "self modification rejected" ($status -eq 400) "status=$status"
}

Write-Output ""
Write-Output "RESULT: $passed passed, $failed failed"
Write-Output "FIRST_RECOVERY_CODE=$recoveryCode"
