# syntax=docker/dockerfile:1.7
#
# Sibling Postgres image for the Urbanist Atlas Fly app.
# Wraps upstream postgres:17-alpine with a tiny root-stage entrypoint
# that pre-chowns the Fly volume mount so the `postgres` user can
# initdb into a subdirectory (workaround for Fly volumes mounting as
# root:root 0755 + ext4 lost+found at the mount root).
#
# Design doc: docs/superpowers/specs/2026-05-21-fly-redeploy-design.md
# Runbook:    docs/deploy.md § Fly: API + sibling Postgres

FROM postgres:17-alpine

COPY entrypoint-fly.sh /usr/local/bin/entrypoint-fly.sh
RUN chmod +x /usr/local/bin/entrypoint-fly.sh

ENTRYPOINT ["/usr/local/bin/entrypoint-fly.sh"]
CMD ["postgres"]
