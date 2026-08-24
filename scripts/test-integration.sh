#!/usr/bin/env sh
set -eu

docker compose up -d db

deadline=$(( $(date +%s) + 60 ))
while :; do
  container_id="$(docker compose ps -q db)"
  if [ -n "$container_id" ]; then
    health="$(docker inspect --format '{{.State.Health.Status}}' "$container_id")"
    if [ "$health" = "healthy" ]; then
      break
    fi
    if [ "$health" = "unhealthy" ]; then
      echo "PostgreSQL test container became unhealthy. Database is preserved for inspection." >&2
      exit 1
    fi
  fi
  if [ "$(date +%s)" -ge "$deadline" ]; then
    echo "Timed out waiting for the PostgreSQL test container. Database is preserved for inspection." >&2
    exit 1
  fi
  sleep 2
done

export TEST_DATABASE_URL='postgres://keystar_test:keystar_test@localhost:5432/keystar_test?sslmode=disable'
cd backend
go test ./tests/integration/... -count=1
