param([string]$DockerContext='desktop-linux')
$ErrorActionPreference='Stop'
Push-Location (Join-Path $PSScriptRoot '..')
try {
 . ./scripts/test-env.ps1
 & ./scripts/contexts.ps1
 & docker --context $DockerContext compose --profile build --profile dev --profile e03-regression build renderer-image generation-api projects-api owner-fixture owner-router storage-api storage-worker jetstream-bootstrap dev-consumer e03-owner-fixture e03-storage-api e03-storage-worker
 if($LASTEXITCODE -ne 0){throw 'Current-source dev regression build failed'}
 $env:GENERATION_RENDERER_IMAGE=& docker --context $DockerContext image inspect leafrun-e04-renderer:source --format '{{.Id}}'
 if($LASTEXITCODE -ne 0 -or $env:GENERATION_RENDERER_IMAGE -notmatch '^sha256:[0-9a-f]{64}$'){throw 'Current immutable dev renderer unavailable'}
 [IO.File]::WriteAllText((Join-Path $infraRoot '.cache/renderer.env'),"GENERATION_RENDERER_IMAGE=$env:GENERATION_RENDERER_IMAGE`n")
 foreach($migration in @('projects-migrate','generation-migrate')) {
  & docker --context $DockerContext compose --profile dev run --rm --no-deps $migration
  if($LASTEXITCODE -ne 0){throw 'Append-only dev regression migration failed'}
 }
 & docker --context $DockerContext compose --profile dev --profile e03-regression up --detach --no-deps --no-build owner-fixture owner-router e03-owner-fixture storage-api storage-worker projects-api projects-reconciler projects-event-relay generation-api generation-workflow generation-dispatcher generation-reconciler generation-controller generation-janitor generation-event-relay dev-consumer e03-storage-api e03-storage-worker
 if($LASTEXITCODE -ne 0){throw 'Current-source dev app refresh failed'}
 foreach($url in @('http://127.0.0.1:58090/readyz','http://127.0.0.1:58091/readyz','http://127.0.0.1:58094/readyz','http://127.0.0.1:58096/readyz')) {
  $ready=$false;for($attempt=0;$attempt -lt 60;$attempt++){try{$r=Invoke-WebRequest $url -TimeoutSec 2 -SkipHttpErrorCheck;if($r.StatusCode -in @(200,204)){$ready=$true;break}}catch{};Start-Sleep -Seconds 1};if(-not $ready){throw 'Current-source dev app readiness failed'}
 }
 Write-Output 'Current-source explicit-dev E03/E04/E05 applications ready; dependency containers and all volumes preserved'
}finally{Pop-Location}
