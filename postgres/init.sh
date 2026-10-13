#!/bin/sh
set -eu
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname postgres \
  -v storage_password="$STORAGE_DB_PASSWORD" -v temporal_password="$TEMPORAL_DB_PASSWORD" <<'SQL'
CREATE ROLE storage LOGIN PASSWORD :'storage_password';
CREATE ROLE temporal LOGIN PASSWORD :'temporal_password';
CREATE DATABASE storage OWNER storage;
CREATE DATABASE storage_test OWNER storage;
CREATE DATABASE temporal OWNER temporal;
CREATE DATABASE temporal_visibility OWNER temporal;
REVOKE CONNECT ON DATABASE storage, storage_test, temporal, temporal_visibility FROM PUBLIC;
GRANT CONNECT ON DATABASE storage, storage_test TO storage;
GRANT CONNECT ON DATABASE temporal, temporal_visibility TO temporal;
SQL
