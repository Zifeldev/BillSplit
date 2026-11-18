#!/usr/bin/env bash
# Runs once on a fresh volume (after 10-roles.sql): sets each service role's
# password from its Docker secret, so no password lives in SQL or compose files.
# For an existing volume run: make db-passwords
set -euo pipefail
for svc in auth social split finance notify; do
  pw="$(cat "/run/secrets/${svc}_db_password")"
  psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" \
    -v role="${svc}_svc" -v pw="$pw" <<'SQL'
ALTER ROLE :"role" PASSWORD :'pw';
SQL
done
echo "service role passwords set from secrets"
