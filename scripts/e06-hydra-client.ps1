param([string]$DockerContext='desktop-linux')
. (Join-Path $PSScriptRoot 'e06-common.ps1') -DockerContext $DockerContext
# This in-network provisioner has no published admin port or client secret.
# A different existing registration fails closed rather than silently widening it.
Invoke-E06Compose run --rm --no-deps --entrypoint /e06-hydra-client oauth-demo
