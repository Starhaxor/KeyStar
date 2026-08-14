$ErrorActionPreference = "Stop"
$staging = "c:\Users\pc\Desktop\Projelerim\KeyStar\.staging\backend"
$target = "C:\Users\pc\Desktop\Projelerim\StarLoader\backend"

$files = @(
    "migrations\000004_phase2.up.sql",
    "migrations\000004_phase2.down.sql",
    "internal\domain\admin.go",
    "internal\security\totp.go",
    "internal\security\totp_test.go",
    "internal\store\admin.go",
    "internal\store\migrations.go",
    "internal\service\adminauth\adminauth.go",
    "internal\service\adminauth\adminauth_test.go",
    "internal\httpapi\admin.go",
    "internal\httpapi\admin_auth.go",
    "internal\httpapi\admin_mfa.go",
    "internal\httpapi\admin_console.go",
    "internal\httpapi\admin_test.go",
    "internal\admin\commands.go",
    "internal\admin\commands_test.go",
    "cmd\server\main.go"
)

foreach ($file in $files) {
    $source = Join-Path $staging $file
    $destination = Join-Path $target $file
    if (-not (Test-Path $source)) { throw "missing staging file: $source" }
    Copy-Item -Path $source -Destination $destination -Force
    Write-Output "copied $file"
}
Write-Output "ALL COPIED"
