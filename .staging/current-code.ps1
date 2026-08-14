param([string]$Secret = '')
if ($Secret -eq '') {
    $Secret = (& docker exec deploy-postgres-1 psql -U postgres -d starloader -t -A -c "select totp_secret from admin_accounts where email='admin@keystar.local'").Trim()
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
$epoch = [long]([DateTimeOffset]::UtcNow.ToUnixTimeSeconds())
$remaining = 30 - ($epoch % 30)
Write-Host "CODE=$(Get-TotpCodeAt $Secret $epoch) NEXT=$(Get-TotpCodeAt $Secret ($epoch + 30)) REMAINING=$remaining"
