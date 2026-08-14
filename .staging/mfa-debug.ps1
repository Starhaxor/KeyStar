$ErrorActionPreference = "Stop"
$base = "http://127.0.0.1:8080/v1/admin"
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

$totpSecret = (& docker exec deploy-postgres-1 psql -U postgres -d starloader -t -A -c "select totp_secret from admin_accounts where email='admin@keystar.local'")
Write-Host "RAW SECRET: [$totpSecret] len=$(if ($totpSecret) { $totpSecret.Length } else { 0 })"
$totpSecret = "$totpSecret".Trim()

$session = New-Object Microsoft.PowerShell.Commands.WebRequestSession
$login = Invoke-WebRequest -Method POST -Uri "$base/auth/login" -ContentType 'application/json' -Body '{"email":"admin@keystar.local","password":"YeniGucluAdmin2026!"}' -WebSession $session -UseBasicParsing
$loginBody = $login.Content | ConvertFrom-Json
Write-Host "LOGIN: $($login.StatusCode) mfa_required=$($loginBody.mfa_required)"

$epoch = [long]([DateTimeOffset]::UtcNow.ToUnixTimeSeconds())
$code = Get-TotpCodeAt $totpSecret $epoch
Write-Host "EPOCH: $epoch CODE: $code"

$mfaBody = '{"mfa_token":"' + $loginBody.mfa_token + '","code":"' + $code + '"}'
try {
    $mfa = Invoke-WebRequest -Method POST -Uri "$base/auth/mfa" -ContentType 'application/json' -Body $mfaBody -WebSession $session -UseBasicParsing
    Write-Host "MFA STATUS: $($mfa.StatusCode) BODY: $($mfa.Content)"
} catch {
    Write-Host "MFA STATUS: $([int]$_.Exception.Response.StatusCode) BODY: $($_.ErrorDetails.Message)"
}
