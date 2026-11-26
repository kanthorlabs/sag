# Layer 4 gate: devx

## DSN host on the development machine

Image used: `docker.io/library/postgres:17.12-bookworm` (present locally, no pull).

Commands and exit codes:

```
# Start the server container
podman run --detach --name dx40-pg --label sag.devx-e2e=1 \
  --env POSTGRES_PASSWORD=<throwaway> \
  --publish 127.0.0.1::5432 \
  docker.io/library/postgres:17.12-bookworm
# exit 0

podman port dx40-pg 5432/tcp
# 127.0.0.1:44681

# Wait for readiness via local socket
podman exec dx40-pg pg_isready -h 127.0.0.1 -p 5432 -U postgres
# 127.0.0.1:5432 - accepting connections; exit 0

# From client container: pg_isready via host.containers.internal
podman run --rm --name dx40-client --label sag.devx-e2e=1 \
  --env PGPASSWORD=<throwaway> \
  docker.io/library/postgres:17.12-bookworm \
  pg_isready -h host.containers.internal -p 44681 -U postgres
# host.containers.internal:44681 - accepting connections; exit 0

# From client container: psql select 1 via host.containers.internal
podman run --rm --name dx40-client --label sag.devx-e2e=1 \
  --env PGPASSWORD=<throwaway> \
  docker.io/library/postgres:17.12-bookworm \
  psql -h host.containers.internal -p 44681 -U postgres -c "select 1"
# returns one row with value 1; exit 0

# Remove server container
podman rm --force --volumes --ignore dx40-pg
# exit 0

# Verify both containers absent (exit 1 = not found)
podman container exists dx40-pg   # exit 1
podman container exists dx40-client  # exit 1
```

Outcome: a container on `podman-machine-default` reaches a PostgreSQL container through `host.containers.internal` and the published host port. `pg_isready` and `psql` both succeed. DX-AC9 can proceed.
