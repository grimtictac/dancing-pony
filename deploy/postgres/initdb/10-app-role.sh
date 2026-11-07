#!/bin/sh
# Creates pony_app, the login role the API connects as. The Postgres image
# runs this once, when the data volume is first initialised; migrations then
# grant the role access to the tables. In production the role is created by
# the first migration and given a password out of band.
set -eu

psql -v ON_ERROR_STOP=1 -v app_password="$PONY_APP_DB_PASSWORD" \
    --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<'SQL'
CREATE ROLE pony_app LOGIN PASSWORD :'app_password';
SQL
