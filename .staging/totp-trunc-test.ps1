$h = [byte[]]@(0x75,0xa4,0x8a,0x19,0xd4,0xcb,0xe1,0x00,0x64,0x4e,0x8a,0xc1,0x39,0x7e,0xea,0x74,0x7a,0x2d,0x33,0xab)
$o = $h[19] -band 0x0F
Write-Output "o=$o type=$($o.GetType().Name)"
Write-Output "index11=$($h[11]) index12=$($h[12]) index13=$($h[13]) index14=$($h[14])"
Write-Output "o+1 expr index=$($h[$o + 1])"
Write-Output "int cast index=$($h[[int]$o + 1])"
$i1 = $o + 1; Write-Output "var i1=$i1 index=$($h[$i1])"
Write-Output "shl direct: $(57 -shl 16)"
Write-Output "shl via arr: $($h[12] -shl 16)"
