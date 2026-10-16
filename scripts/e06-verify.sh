#!/bin/sh
# Invoked inside the owned, read-only-source Linux verifier.
set -eu
export GOWORK=/tmp/e06-qualification.work
export GOCACHE=/root/.cache/go-build GOMODCACHE=/go/pkg/mod
export GOFLAGS=-buildvcs=false CGO_ENABLED=1
export PATH=/tools:$PATH
case "$(typst --version)" in 'typst 0.15.1 '*) ;; *) echo 'Expected Typst 0.15.1' >&2; exit 1;; esac
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
for repo in contracts platform access audit core integrations storage projects generation infra; do
  git -C /tmp config --global --add safe.directory "/work/$repo"
  cd "/work/$repo"
  go test -race -json -count=1 -timeout=15m ./...
  go vet ./...
  go build -buildvcs=false ./...
  printf '{"qualificationModule":"%s","raceVetBuild":"passed"}\n' "$repo"
done
