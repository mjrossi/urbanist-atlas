#!/bin/sh
# Wraps the upstream postgres:17-alpine docker-entrypoint.sh to work
# around two Fly-specific quirks of volume mounts:
#
#   1. Fly's ext4 volumes auto-include a `lost+found` directory at
#      the mount root, which trips initdb's "directory exists but is
#      not empty" guard. We sidestep that by pointing PGDATA at a
#      SUBDIRECTORY of the mount (`/var/lib/postgresql/data/pgdata`,
#      set in infra/postgres/fly.toml [env]), so initdb has a clean
#      target inside the volume.
#
#   2. Fly mounts volumes as `root:root mode 0755`. The upstream
#      postgres entrypoint demotes itself to the `postgres` user
#      very early, before it can create the PGDATA subdir — and
#      `postgres` can't write into a root-owned mount root.
#
# Solution: while still running as root (Fly init starts us as root),
# pre-create the PGDATA subdir and chown the mount root to postgres,
# then hand off to docker-entrypoint.sh which will do its own
# demotion + initdb.
set -e

if [ -n "$PGDATA" ]; then
    mkdir -p "$PGDATA"
    chown -R postgres:postgres "$(dirname "$PGDATA")"
fi

exec docker-entrypoint.sh "$@"
