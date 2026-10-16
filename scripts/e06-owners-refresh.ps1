param([string]$DockerContext='desktop-linux',[string[]]$Services=@('access-api-a','access-api-b','access-reconcile','owner-router','storage-api','storage-worker','projects-api','projects-reconciler','generation-api','generation-workflow','generation-dispatcher','generation-reconciler','generation-controller','generation-janitor'))
. (Join-Path $PSScriptRoot 'e06-common.ps1') -DockerContext $DockerContext
$e06AllowedServices=@('access-api-a','access-api-b','access-reconcile','owner-router','storage-api','storage-worker','projects-api','projects-reconciler','generation-api','generation-workflow','generation-dispatcher','generation-reconciler','generation-controller','generation-janitor')
if(-not $Services.Count -or @($Services|Where-Object{$_ -notin $e06AllowedServices}).Count){throw 'Only owned E06 application services may be refreshed'}
$e06Renderer=& docker --context $DockerContext image inspect leafrun-e06-renderer:source --format '{{.Id}}'
if($LASTEXITCODE -ne 0 -or $e06Renderer -notmatch '^sha256:[0-9a-f]{64}$'){throw 'Current immutable E06 renderer image unavailable'}
$env:E06_RENDERER_IMAGE=$e06Renderer
Invoke-E06Compose up --detach --no-deps --no-build @Services
