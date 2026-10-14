#!/bin/sh
set -eu
for attempt in $(seq 1 60); do
  if temporal operator cluster health --address temporal:7233 >/dev/null 2>&1; then
    for namespace in default e03-regression e03-unit leafrun-generation; do
      if ! temporal operator namespace describe --address temporal:7233 --namespace "$namespace" >/dev/null 2>&1; then
        temporal operator namespace create --address temporal:7233 --namespace "$namespace" --retention 30d
      fi
    done
    exit 0
  fi
  sleep 2
done
echo 'Temporal did not become ready' >&2
exit 1
