#!/bin/sh
set -eu
install -m 755 /tools/typst /usr/local/bin/typst
typst --version
command -v git
command -v ssh
cat > /tmp/e03.work <<'WORK'
go 1.27.0
use (
 /work/core
 /work/contracts
 /work/storage
 /work/infra
 /work/integrations
)
replace github.com/leafrun/leafrun-contracts v0.1.0 => /work/contracts
replace github.com/leafrun/leafrun-document-core v0.1.0 => /work/core
replace github.com/leafrun/leafrun-integrations v0.1.0 => /work/integrations
WORK
export GOWORK=/tmp/e03.work
for repo in ${LEAFRUN_E03_CHECK_REPOS:-core contracts storage infra}; do
  case "$repo" in core|contracts|storage|infra) ;; *) echo 'Invalid repository' >&2; exit 1;; esac
  echo "Verifying $repo"
  cd "/work/$repo"
  go test -race ./... -count=1
  go vet ./...
  go build -buildvcs=false ./...
done
