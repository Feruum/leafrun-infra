#!/bin/sh
set -eu
for attempt in $(seq 1 60); do
  if temporal operator cluster health --address temporal:7233 >/dev/null 2>&1; then
    if ! temporal operator namespace describe --address temporal:7233 --namespace default >/dev/null 2>&1; then
      temporal operator namespace create --address temporal:7233 --namespace default --retention 30d
    fi
    exit 0
  fi
  sleep 2
done
echo 'Temporal did not become ready' >&2
exit 1
