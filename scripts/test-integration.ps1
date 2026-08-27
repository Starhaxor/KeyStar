$ErrorActionPreference = "Stop"

docker compose up -d db
if ($LASTEXITCODE -ne 0) {
  throw "Could not start the PostgreSQL test container."
}

$deadline = [DateTime]::UtcNow.AddSeconds(60)
while ($true) {
  $containerID = (docker compose ps -q db).Trim()
  if ($LASTEXITCODE -ne 0) {
    throw "Could not inspect the PostgreSQL test container."
  }
  if ($containerID) {
    $health = (docker inspect --format '{{.State.Health.Status}}' $containerID).Trim()
    if ($LASTEXITCODE -ne 0) {
      throw "Could not inspect the PostgreSQL test container health."
    }
    if ($health -eq "healthy") {
      break
    }
    if ($health -eq "unhealthy") {
      throw "PostgreSQL test container became unhealthy. Database is preserved for inspection."
    }
  }
  if ([DateTime]::UtcNow -ge $deadline) {
    throw "Timed out waiting for the PostgreSQL test container. Database is preserved for inspection."
  }
  Start-Sleep -Seconds 2
}

$env:TEST_DATABASE_URL = "postgres://keystar_test:keystar_test@127.0.0.1:55432/keystar_test?sslmode=disable"
Push-Location backend
try {
  go test ./tests/integration/... -count=1
  $testExitCode = $LASTEXITCODE
  if ($testExitCode -ne 0) {
    throw "Integration tests failed with exit code $testExitCode. Database is preserved for inspection."
  }
} finally {
  Pop-Location
}
