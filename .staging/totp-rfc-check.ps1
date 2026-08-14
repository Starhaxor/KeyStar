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

$secret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
$vectors = @(
    @(59, "287082"),
    @(1111111109, "081804"),
    @(1111111111, "050471"),
    @(1234567890, "005924"),
    @(2000000000, "279037")
)
foreach ($vector in $vectors) {
    $got = Get-TotpCodeAt $secret $vector[0]
    if ($got -eq $vector[1]) { Write-Output "PASS t=$($vector[0]) code=$got" }
    else { Write-Output "FAIL t=$($vector[0]) got=$got want=$($vector[1])" }
}
