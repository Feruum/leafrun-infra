#!/bin/sh
set -eu
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname postgres \
  -v storage_password="$STORAGE_DB_PASSWORD" -v temporal_password="$TEMPORAL_DB_PASSWORD" \
  -v generation_password="$GENERATION_DB_PASSWORD" -v consumer_password="$CONSUMER_DB_PASSWORD" \
  -v projects_password="$PROJECTS_DB_PASSWORD" <<'SQL'
CREATE ROLE storage LOGIN PASSWORD :'storage_password';
CREATE ROLE temporal LOGIN PASSWORD :'temporal_password';
CREATE ROLE generation LOGIN PASSWORD :'generation_password';
CREATE ROLE projects LOGIN PASSWORD :'projects_password';
CREATE ROLE dev_consumer LOGIN PASSWORD :'consumer_password';
CREATE DATABASE storage OWNER storage;
CREATE DATABASE storage_test OWNER storage;
CREATE DATABASE storage_regression OWNER storage;
CREATE DATABASE temporal OWNER temporal;
CREATE DATABASE temporal_visibility OWNER temporal;
CREATE DATABASE generation OWNER generation;
CREATE DATABASE generation_test OWNER generation;
CREATE DATABASE projects OWNER projects;
CREATE DATABASE projects_test OWNER projects;
CREATE DATABASE dev_consumer OWNER dev_consumer;
CREATE DATABASE platform_test OWNER dev_consumer;
REVOKE CONNECT ON DATABASE storage, storage_test, storage_regression, temporal, temporal_visibility, generation, generation_test, projects, projects_test, dev_consumer, platform_test FROM PUBLIC;
GRANT CONNECT ON DATABASE storage, storage_test, storage_regression TO storage;
GRANT CONNECT ON DATABASE temporal, temporal_visibility TO temporal;
GRANT CONNECT ON DATABASE generation, generation_test TO generation;
GRANT CONNECT ON DATABASE projects, projects_test TO projects;
GRANT CONNECT ON DATABASE dev_consumer, platform_test TO dev_consumer;
SQL
