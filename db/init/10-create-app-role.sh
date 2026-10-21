#!/bin/sh
set -eu

: "${APP_DB_USER:?APP_DB_USER requis}"
: "${APP_DB_PASSWORD:?APP_DB_PASSWORD requis}"

psql --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" \
  --set=ON_ERROR_STOP=1 \
  --set=app_user="$APP_DB_USER" \
  --set=app_password="$APP_DB_PASSWORD" <<'SQL'

SELECT format(
    'CREATE ROLE %I WITH LOGIN PASSWORD %L NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOINHERIT',
    :'app_user',
    :'app_password'
)
WHERE NOT EXISTS (
    SELECT 1 FROM pg_roles WHERE rolname = :'app_user'
)
\gexec

SELECT format(
    'CREATE SCHEMA IF NOT EXISTS %I AUTHORIZATION %I',
    'edimaroc_app',
    :'app_user'
)
\gexec

SELECT format(
    'ALTER ROLE %I SET search_path TO edimaroc_app',
    :'app_user'
)
\gexec

SELECT format(
    'GRANT CONNECT ON DATABASE %I TO %I',
    current_database(),
    :'app_user'
)
\gexec

REVOKE CREATE ON SCHEMA public FROM PUBLIC;
SQL