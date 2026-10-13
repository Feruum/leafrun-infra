param([string]$DockerContext='desktop-linux')
$ErrorActionPreference='Stop'
Push-Location (Join-Path $PSScriptRoot '..')
try {
 & ./scripts/init-env.ps1
 & ./scripts/contexts.ps1
 docker --context $DockerContext compose config --quiet
 if($LASTEXITCODE -ne 0){throw 'Compose validation failed'}
 docker --context $DockerContext compose up -d postgres seaweed temporal-namespace
 if($LASTEXITCODE -ne 0){throw 'Dependency startup failed'}
 . ./scripts/test-env.ps1
 $env:AWS_ACCESS_KEY_ID=$cfg.S3_ADMIN_ACCESS_KEY
 $env:AWS_SECRET_ACCESS_KEY=$cfg.S3_ADMIN_SECRET_KEY
 go run ./cmd/bootstrap-s3
 if($LASTEXITCODE -ne 0){throw 'Versioning bootstrap failed'}
 $env:AWS_ACCESS_KEY_ID=$cfg.S3_ACCESS_KEY
 $env:AWS_SECRET_ACCESS_KEY=$cfg.S3_SECRET_KEY
 docker --context $DockerContext compose --profile dev up -d --build storage-api storage-worker
 if($LASTEXITCODE -ne 0){throw 'Storage startup failed'}
} finally {Pop-Location}
