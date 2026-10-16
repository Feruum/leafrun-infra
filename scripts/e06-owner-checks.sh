#!/bin/sh
set -eu
export GOWORK=/tmp/e06-owner.work
export GOCACHE=/root/.cache/go-build
export GOMODCACHE=/go/pkg/mod
export PATH=/tools:$PATH
cat > "$GOWORK" <<'WORK'
go 1.27.0
use (
 /work/infra
 /work/access
 /work/audit
 /work/contracts
 /work/platform
 /work/core
 /work/generation
 /work/projects
 /work/storage
 /work/integrations
)
replace github.com/leafrun/leafrun-contracts v0.1.0 => /work/contracts
replace github.com/leafrun/leafrun-platform v0.1.0 => /work/platform
replace github.com/leafrun/leafrun-document-core v0.1.0 => /work/core
replace github.com/leafrun/leafrun-integrations v0.1.0 => /work/integrations
WORK
cd /work/infra
exec go test -buildvcs=false -count=1 -timeout=15m "$@"
