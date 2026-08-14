$alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"
$base32Secret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
$bits = ""
foreach ($char in $base32Secret.ToUpper().ToCharArray()) {
    $index = $alphabet.IndexOf($char)
    $bits += [Convert]::ToString($index, 2).PadLeft(5, '0')
}
Write-Output ("bits length = " + $bits.Length)
$bytes = New-Object System.Collections.Generic.List[byte]
for ($i = 0; $i + 8 -le $bits.Length; $i += 8) {
    $bytes.Add([Convert]::ToByte($bits.Substring($i, 8), 2))
}
Write-Output ("decoded key = " + [System.Text.Encoding]::ASCII.GetString($bytes.ToArray()))

$counter = 1L
$counterBytes = New-Object byte[] 8
$counterBytes[7] = 1
$hmac = New-Object System.Security.Cryptography.HMACSHA1
$hmac.Key = $bytes.ToArray()
$hash = $hmac.ComputeHash($counterBytes)
$hex = ($hash | ForEach-Object { $_.ToString("x2") }) -join ""
Write-Output ("hmac = " + $hex)
# RFC 4226 appendix D for counter=1: HMAC-SHA1 = 102ad56dbf7cf0cd4114544954e4dd642a4a1b31
$offset = $hash[19] -band 0x0F
Write-Output ("offset = " + $offset)
$value = (($hash[$offset] -band 0x7F) -shl 24) -bor ($hash[$offset + 1] -shl 16) -bor ($hash[$offset + 2] -shl 8) -bor $hash[$offset + 3]
Write-Output ("value = " + $value + " code = " + ($value % 1000000).ToString("D6"))
