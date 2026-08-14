# Final cleanup: fix .env, purge test artifacts, rotate admin credentials.
Set-Location "C:\Users\pc\Desktop\Projelerim\StarLoader\backend"

# 1. Fix DATABASE_URL permanently (compose binds Postgres to 127.0.0.1 only).
$envPath = ".\.env"
$content = Get-Content $envPath -Raw
$content = $content -replace 'host\.docker\.internal:55432', '127.0.0.1:55432'
Set-Content $envPath -Value $content -NoNewline
Write-Host "[1] .env DATABASE_URL fixed:"
(Get-Content $envPath | Select-String 'DATABASE_URL')

# 2. Purge e2e test artifacts (tester@example.com + orphaned rows).
docker exec deploy-postgres-1 psql -U postgres -d starloader -c "delete from auth_sessions where user_id in (select id from users where email='tester@example.com');"
docker exec deploy-postgres-1 psql -U postgres -d starloader -c "delete from devices where user_id in (select id from users where email='tester@example.com');"
docker exec deploy-postgres-1 psql -U postgres -d starloader -c "delete from licenses where user_id in (select id from users where email='tester@example.com');"
docker exec deploy-postgres-1 psql -U postgres -d starloader -c "delete from users where email='tester@example.com';"
Write-Host "[2] test artifacts purged"

# 3. Rotate admin credentials: drop the old account, recreate with a new password.
docker exec deploy-postgres-1 psql -U postgres -d starloader -c "delete from admin_sessions where admin_account_id in (select id from admin_accounts where email='admin@keystar.local');"
docker exec deploy-postgres-1 psql -U postgres -d starloader -c "delete from admin_accounts where email='admin@keystar.local';"

Get-Content .env | ForEach-Object {
  if ($_ -match '^\s*([^#=\s][^=]*)=(.*)$') {
    [Environment]::SetEnvironmentVariable($matches[1].Trim(), $matches[2].Trim(), 'Process')
  }
}

$password = 'YeniGucluAdmin2026!'
$pipeText = "{0}`n{0}" -f $password
$pipeText | go run ./cmd/server admin create-admin --email admin@keystar.local --password-stdin
Write-Host "[3] admin recreated, exit: $LASTEXITCODE"

docker exec deploy-postgres-1 psql -U postgres -d starloader -c "select email, status from admin_accounts; select count(*) as users from users; select count(*) as licenses from licenses; select count(*) as sessions from auth_sessions;"
