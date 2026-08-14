$ErrorActionPreference = "Stop"
$staging = "C:\Users\pc\Desktop\Projelerim\KeyStar\.staging\backend"
$target  = "C:\Users\pc\Desktop\Projelerim\StarLoader\backend"
$files = @(
    "internal\domain\console.go",
    "internal\domain\errors.go",
    "internal\store\console.go",
    "internal\store\admin.go",
    "internal\store\users.go",
    "internal\httpapi\admin.go",
    "internal\httpapi\admin_console.go",
    "internal\httpapi\admin_mfa.go"
)
foreach ($f in $files) {
    Copy-Item -Path (Join-Path $staging $f) -Destination (Join-Path $target $f) -Force
    Write-Host "copied $f"
}
Set-Location $target
go build ./...
if ($LASTEXITCODE -ne 0) { Write-Host "BUILD FAILED"; exit 1 }
go vet ./...
if ($LASTEXITCODE -ne 0) { Write-Host "VET FAILED"; exit 1 }
Write-Host "BUILD+VET OK"
