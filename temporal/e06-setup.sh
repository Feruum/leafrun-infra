#!/bin/sh
set -eu
# The normal-user startup script probes these owned databases before invoking
# this migrator. Unknown existing state must not reset schema-version metadata.
for database in temporal temporal_visibility; do
  if [ "$database" = temporal ]; then present="$E06_TEMPORAL_SCHEMA_EXISTS"; schema=temporal; else present="$E06_VISIBILITY_SCHEMA_EXISTS"; schema=visibility; fi
  export SQL_DATABASE="$database"
  case "$present" in
    false) temporal-sql-tool setup-schema -v 0.0 ;;
    true) ;;
    *) echo 'Explicit owned Temporal schema probe required' >&2; exit 1 ;;
  esac
  temporal-sql-tool update-schema -d "/etc/temporal/schema/postgresql/v12/$schema/versioned"
done
