param([string]$DockerContext='desktop-linux',[switch]$Build)
. (Join-Path $PSScriptRoot 'e06-common.ps1') -DockerContext $DockerContext
& (Join-Path $PSScriptRoot 'e06-init-env.ps1') -DockerContext $DockerContext
& (Join-Path $PSScriptRoot 'e06-domain-database.ps1') -DockerContext $DockerContext
Invoke-E06Compose config --quiet
if($Build){& (Join-Path $PSScriptRoot 'contexts.ps1');Invoke-E06Compose build audit-migrate audit-stream-bootstrap}
Invoke-E06Compose up --detach --wait --no-deps nats
# Provision the bounded stream and fixed durable before any producer is started.
Invoke-E06Compose run --rm --no-deps audit-stream-bootstrap
Invoke-E06Compose run --rm --no-deps audit-migrate
Invoke-E06Compose up --detach --no-deps --no-build audit-api audit-consumer audit-cleanup access-event-relay generation-event-relay projects-event-relay storage-event-relay
$e06AuditReady=$false
for($e06Attempt=0;$e06Attempt -lt 60;$e06Attempt++){
 try {$e06Response=Invoke-WebRequest 'http://127.0.0.1:58199/readyz' -TimeoutSec 2 -SkipHttpErrorCheck;if($e06Response.StatusCode -eq 200){$e06AuditReady=$true;break}}catch{}
 Start-Sleep -Seconds 1
}
if(-not $e06AuditReady){throw 'E06 Audit readiness failed'}
Write-Output 'Owned E06 Audit API/consumer/finite cleanup and four authenticated owner relays started; NATS and Audit ports are loopback only'
