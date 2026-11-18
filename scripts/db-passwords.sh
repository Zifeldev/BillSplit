#!/usr/bin/env sh
# Applies passwords from ./secrets to an EXISTING Postgres volume (fresh volumes
# get them automatically from deploy/postgres/20-role-passwords.sh). Data is untouched.
set -eu
cd "$(dirname "$0")/.."
apply() { # role secret-file
  docker compose exec -T postgres psql -v ON_ERROR_STOP=1 -U billsplit -d billsplit \
    -v role="$1" -v pw="$(cat "secrets/$2")" <<'SQL'
ALTER ROLE :"role" PASSWORD :'pw';
SQL
}
apply billsplit postgres_password
for svc in auth social split finance notify; do apply "${svc}_svc" "${svc}_db_password"; done
echo "role passwords updated"
