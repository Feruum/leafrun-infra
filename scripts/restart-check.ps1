param([string]$DockerContext='desktop-linux')
$ErrorActionPreference='Stop'
Push-Location (Join-Path $PSScriptRoot '..')
try {
 . ./scripts/test-env.ps1
 $env:LEAFRUN_E03_RECEIPT=Join-Path $infraRoot '.cache/restart-receipt.json'
 go test -count=1 -v ./e03 -run '^TestRoundTripAndPublication$'
 if($LASTEXITCODE -ne 0){throw 'Initial round trip failed'}
 docker --context $DockerContext compose restart postgres seaweed temporal storage-api storage-worker
 if($LASTEXITCODE -ne 0){throw 'Stand restart failed'}
 $ready=$false
 for($attempt=0;$attempt -lt 60;$attempt++){
  try { $response=Invoke-WebRequest 'http://127.0.0.1:58081/readyz' -TimeoutSec 2 -SkipHttpErrorCheck; if($response.StatusCode -eq 204 -or $response.StatusCode -eq 200){$ready=$true;break} } catch {}
  Start-Sleep -Seconds 1
 }
 if(-not $ready){throw 'Storage did not recover after restart'}
 $env:LEAFRUN_E03_RESTART_RECEIPT=$env:LEAFRUN_E03_RECEIPT
 Remove-Item Env:LEAFRUN_E03_RECEIPT
 go test -count=1 -v ./e03 -run '^TestRestartPersistence$'
 if($LASTEXITCODE -ne 0){throw 'Persistent receipt/object check failed'}
 docker --context $DockerContext compose run --rm temporal-namespace
 if($LASTEXITCODE -ne 0){throw 'Persistent Temporal namespace unavailable'}
} finally {Remove-Item Env:LEAFRUN_E03_RECEIPT,Env:LEAFRUN_E03_RESTART_RECEIPT -ErrorAction SilentlyContinue;Pop-Location}
