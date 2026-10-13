#!/bin/sh
set -eu
for database in temporal temporal_visibility; do
  export SQL_DATABASE="$database"
  temporal-sql-tool setup-schema -v 0.0
  if [ "$database" = temporal ]; then schema=temporal; else schema=visibility; fi
  temporal-sql-tool update-schema -d "/etc/temporal/schema/postgresql/v12/$schema/versioned"
done
