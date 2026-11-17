#!/bin/sh
# Only runs for a NEW database volume. Existing deployments must migrate roles
# explicitly; never delete a production volume to make this script run again.
set -eu
psql --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" --set=ON_ERROR_STOP=1 --set=app_password="$RENTAL_DB_PASSWORD" <<'SQL'
REVOKE ALL ON DATABASE rental_atlas FROM PUBLIC;
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
CREATE ROLE rental_app LOGIN PASSWORD :'app_password' NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS CONNECTION LIMIT 16;
GRANT CONNECT ON DATABASE rental_atlas TO rental_app;
CREATE SCHEMA rental AUTHORIZATION rental_app;
ALTER ROLE rental_app IN DATABASE rental_atlas SET search_path = rental, pg_catalog;
ALTER ROLE rental_app SET statement_timeout = '15s';
ALTER ROLE rental_app SET lock_timeout = '5s';
ALTER ROLE rental_app SET idle_in_transaction_session_timeout = '30s';
SQL
