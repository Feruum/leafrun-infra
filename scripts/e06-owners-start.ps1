param([string]$DockerContext='desktop-linux',[switch]$Build)
. (Join-Path $PSScriptRoot 'e06-common.ps1') -DockerContext $DockerContext
& (Join-Path $PSScriptRoot 'e06-init-env.ps1') -DockerContext $DockerContext
& (Join-Path $PSScriptRoot 'e06-domain-database.ps1') -DockerContext $DockerContext
Invoke-E06Compose --profile build config --quiet
if($Build){& (Join-Path $PSScriptRoot 'contexts.ps1');Invoke-E06Compose --profile build build access-migrate storage-migrate projects-migrate generation-migrate owner-router renderer-image audit-migrate audit-stream-bootstrap}
$e06Renderer=& docker --context $DockerContext image inspect leafrun-e06-renderer:source --format '{{.Id}}'
if($LASTEXITCODE -ne 0 -or $e06Renderer -notmatch '^sha256:[0-9a-f]{64}$'){throw 'Fresh immutable E06 renderer image unavailable'}
$env:E06_RENDERER_IMAGE=$e06Renderer
$e06Values=Read-E06Secrets
try {
 $e06Postgres=Get-E06Container postgres
 foreach($e06Database in @('temporal','temporal_visibility')){
  $e06SchemaExists=& docker --context $DockerContext exec -e "PGPASSWORD=$($e06Values.E06_TEMPORAL_DB_PASSWORD)" $e06Postgres psql -h 127.0.0.1 --username temporal --dbname $e06Database --tuples-only --no-align -v ON_ERROR_STOP=1 -c "SELECT to_regclass('public.schema_version') IS NOT NULL" 2>$null
  if($LASTEXITCODE -ne 0 -or $e06SchemaExists -notin @('t','f')){throw 'Owned Temporal schema probe failed'}
  $e06SchemaVariable=if($e06Database -eq 'temporal'){'E06_TEMPORAL_SCHEMA_EXISTS'}else{'E06_VISIBILITY_SCHEMA_EXISTS'}
  [Environment]::SetEnvironmentVariable($e06SchemaVariable,([bool]($e06SchemaExists -eq 't')).ToString().ToLowerInvariant(),'Process')
 }
 Invoke-E06Compose run --rm --no-deps seaweed-config-init
 Invoke-E06Compose up --detach --no-deps seaweed
 Invoke-E06Compose run --rm --no-deps temporal-schema
 Invoke-E06Compose up --detach --no-deps temporal
 Invoke-E06Compose run --rm --no-deps temporal-namespace
 $env:AWS_ACCESS_KEY_ID=$e06Values.E06_S3_ADMIN_ACCESS_KEY;$env:AWS_SECRET_ACCESS_KEY=$e06Values.E06_S3_ADMIN_SECRET_KEY;$env:AWS_REGION='us-east-1';$env:AWS_EC2_METADATA_DISABLED='true'
 $env:LEAFRUN_DEV_FIXTURE='true';$env:LEAFRUN_STORAGE_S3_ENDPOINT='http://127.0.0.1:58643';$env:LEAFRUN_STORAGE_S3_BUCKET='leafrun-e06-storage'
 Push-Location $e06InfraRoot
 try {
  & go run ./cmd/bootstrap-s3
  if($LASTEXITCODE -ne 0){
   $e06S3Container=Invoke-E06Compose ps --all -q seaweed
   if($e06S3Container -and @($e06S3Container).Count -eq 1){
    $e06S3Project=& docker --context $DockerContext inspect --format '{{index .Config.Labels "com.docker.compose.project"}}' $e06S3Container
    if($LASTEXITCODE -eq 0 -and $e06S3Project -eq 'leafrun-e06'){
     & docker --context $DockerContext inspect --format 'S3 state={{.State.Status}} exit={{.State.ExitCode}} oom={{.State.OOMKilled}} restarts={{.RestartCount}}' $e06S3Container
     $e06S3Logs=(& docker --context $DockerContext logs --tail 150 $e06S3Container 2>&1)-join "`n"
     @{s3PermissionDenied=($e06S3Logs -match 'permission denied');s3DiskFull=($e06S3Logs -match 'no space left');s3InvalidFlag=($e06S3Logs -match 'flag provided but not defined');s3InvalidJSON=($e06S3Logs -match 'invalid character|unexpected end of JSON')}|ConvertTo-Json -Compress|Write-Output
     Remove-Variable e06S3Logs
    }
   }
   throw 'E06 private bucket capability bootstrap failed'
  }
 }finally{Pop-Location}
 foreach($e06Migration in @('storage-migrate','projects-migrate','generation-migrate','access-migrate')){Invoke-E06Compose run --rm --no-deps $e06Migration}
 & (Join-Path $PSScriptRoot 'e06-owners-refresh.ps1') -DockerContext $DockerContext
 & (Join-Path $PSScriptRoot 'e06-audit-start.ps1') -DockerContext $DockerContext
 foreach($e06Address in @('http://127.0.0.1:58197/readyz','http://127.0.0.1:58198/readyz','http://127.0.0.1:58290/readyz','http://127.0.0.1:58291/readyz','http://127.0.0.1:58296/readyz')){
  $e06Ready=$false;for($e06Attempt=0;$e06Attempt -lt 60;$e06Attempt++){try{$e06Response=Invoke-WebRequest $e06Address -TimeoutSec 2 -SkipHttpErrorCheck;if($e06Response.StatusCode -in @(200,204)){$e06Ready=$true;break}}catch{};Start-Sleep -Seconds 1};if(-not $e06Ready){throw 'E06 owner readiness failed'}
 }
 Write-Output 'Owned E06 current-authority domain stand ready; existing E04 services and all volumes preserved'
}finally{Remove-Variable e06Values -ErrorAction SilentlyContinue;Remove-Item Env:AWS_ACCESS_KEY_ID,Env:AWS_SECRET_ACCESS_KEY -ErrorAction SilentlyContinue}
