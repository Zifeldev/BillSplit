#!/usr/bin/env sh
# Creates ./secrets/* for docker-compose secrets (mounted at /run/secrets/<name>).
# Existing files are never overwritten, so it's safe to re-run. Optional API keys
# are created empty — paste your key into the file to enable the feature.
set -eu
cd "$(dirname "$0")/.."
mkdir -p secrets
chmod 700 secrets

rand() { openssl rand -base64 48 | tr -d '\n/+=' | cut -c1-40; }

gen() { # name [value]
  f="secrets/$1"
  if [ -s "$f" ] || { [ -e "$f" ] && [ "${2-x}" = "" ]; }; then
    return
  fi
  if [ $# -ge 2 ]; then printf '%s' "$2" > "$f"; else rand > "$f"; fi
  chmod 600 "$f"
  echo "created $f"
}

gen jwt_secret
gen internal_token
gen postgres_password          # superuser (admin only, services never use it)
for svc in auth social split finance notify; do
  gen "${svc}_db_password"     # one Postgres role per service
done
gen redis_password
gen grafana_admin_password

# Optional third-party keys: empty = feature disabled.
gen ocrspace_api_key ""
gen anthropic_api_key ""
gen expo_access_token ""

echo "secrets ready in ./secrets (git-ignored). Optional keys: ocrspace_api_key, anthropic_api_key, expo_access_token"
