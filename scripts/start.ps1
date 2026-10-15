param([string]$DockerContext='desktop-linux')
$ErrorActionPreference='Stop'
Push-Location (Join-Path $PSScriptRoot '..')
try {
 & ./scripts/init-env.ps1
 & ./scripts/contexts.ps1
 . ./scripts/test-env.ps1
 docker --context $DockerContext compose --profile dev --profile e03-regression config --quiet
 if($LASTEXITCODE -ne 0){throw 'Compose validation failed'}
 docker --context $DockerContext compose --profile dev up -d postgres seaweed nats temporal-namespace
 if($LASTEXITCODE -ne 0){throw 'Dependency startup failed'}
 & ./scripts/projects-database.ps1 -DockerContext $DockerContext
 docker --context $DockerContext compose --profile dev run --rm temporal-namespace
 if($LASTEXITCODE -ne 0){throw 'Temporal namespaces were not initialized'}
 $env:AWS_ACCESS_KEY_ID=$cfg.S3_ADMIN_ACCESS_KEY
 $env:AWS_SECRET_ACCESS_KEY=$cfg.S3_ADMIN_SECRET_KEY
 go run ./cmd/bootstrap-s3
 if($LASTEXITCODE -ne 0){throw 'Versioning bootstrap failed'}
 $env:AWS_ACCESS_KEY_ID=$cfg.S3_TEST_ACCESS_KEY
 $env:AWS_SECRET_ACCESS_KEY=$cfg.S3_TEST_SECRET_KEY
 docker --context $DockerContext compose --profile build --profile dev --profile e03-regression build --quiet renderer-image generation-api projects-api owner-fixture owner-router storage-api storage-worker jetstream-bootstrap dev-consumer e03-owner-fixture e03-storage-api e03-storage-worker
 if($LASTEXITCODE -ne 0){throw 'E04 image build failed'}
 $env:GENERATION_RENDERER_IMAGE=docker --context $DockerContext image inspect leafrun-e04-renderer:source --format '{{.Id}}'
 if($LASTEXITCODE -ne 0 -or $env:GENERATION_RENDERER_IMAGE -notmatch '^sha256:[0-9a-f]{64}$'){throw 'Cannot resolve immutable renderer image'}
 [IO.File]::WriteAllText((Join-Path $infraRoot '.cache/renderer.env'),"GENERATION_RENDERER_IMAGE=$env:GENERATION_RENDERER_IMAGE`n")
 docker --context $DockerContext compose --profile dev --profile e03-regression up -d --no-build storage-api storage-worker projects-api projects-reconciler projects-event-relay generation-api generation-workflow generation-dispatcher generation-reconciler generation-controller generation-janitor generation-event-relay dev-consumer e03-storage-api e03-storage-worker
 if($LASTEXITCODE -ne 0){throw 'E04 process startup failed'}
 foreach($address in @('http://127.0.0.1:58090/readyz','http://127.0.0.1:58091/readyz','http://127.0.0.1:58094/readyz','http://127.0.0.1:58096/readyz')){
  $ready=$false
  for($attempt=0;$attempt -lt 60;$attempt++){
   try{$response=Invoke-WebRequest $address -TimeoutSec 2 -SkipHttpErrorCheck;if($response.StatusCode -eq 204 -or $response.StatusCode -eq 200){$ready=$true;break}}catch{}
   Start-Sleep -Seconds 1
  }
  if(-not $ready){throw "Service failed readiness: $address"}
 }
} finally {Pop-Location}
