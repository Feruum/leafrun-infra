#!/bin/sh
set -eu
install -m 755 /tools/typst /usr/local/bin/typst
typst --version
command -v git
command -v ssh
cat > /tmp/e05.work <<'WORK'
go 1.27.0
use (
 /work/core
 /work/contracts
 /work/storage
 /work/generation
 /work/projects
 /work/platform
 /work/infra
 /work/integrations
)
replace github.com/leafrun/leafrun-contracts v0.1.0 => /work/contracts
replace github.com/leafrun/leafrun-document-core v0.1.0 => /work/core
replace github.com/leafrun/leafrun-integrations v0.1.0 => /work/integrations
replace github.com/leafrun/leafrun-platform v0.1.0 => /work/platform
WORK
export GOWORK=/tmp/e05.work
export GOFLAGS=-buildvcs=false
# Host checkouts belong to the runner UID while this verifier runs as root.
# Nested Go builds also inspect Git, so trust only these fixed workspace roots.
for source in core contracts storage generation projects platform infra integrations; do
  git config --global --add safe.directory "/work/$source"
done
for repo in ${LEAFRUN_E03_CHECK_REPOS:-core contracts storage generation projects platform infra}; do
  case "$repo" in core|contracts|storage|generation|projects|platform|infra) ;; *) echo 'Invalid repository' >&2; exit 1;; esac
  echo "Verifying $repo"
  cd "/work/$repo"
  go test -race ./... -count=1
  go vet ./...
  go build -buildvcs=false ./...
done
