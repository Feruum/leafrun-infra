param([string]$DockerContext='desktop-linux')
$ErrorActionPreference='Stop'
Push-Location (Join-Path $PSScriptRoot '..')
try {
 . ./scripts/test-env.ps1
 $env:LEAFRUN_E03_RECEIPT=Join-Path $infraRoot '.cache/restart-receipt.json'
 go test -count=1 -v ./e03 -run '^TestRoundTripAndPublication$'
 if($LASTEXITCODE -ne 0){throw 'Initial round trip failed'}
 $env:LEAFRUN_E04_RECEIPT_FILE=Join-Path $infraRoot '.cache/e04-restart-receipt.json'
 go test -count=1 -v ./e04 -run '^TestLiveJobParametersPublicationAndAccess$'
 if($LASTEXITCODE -ne 0){throw 'Initial Generation publication failed'}
 $env:LEAFRUN_E05_INTEGRATION='1'
 $env:LEAFRUN_E05_RECEIPT_FILE=Join-Path $infraRoot '.cache/e05-restart-receipt.json'
 go test -count=1 -v ./e04 -run '^TestE05ReleaseSelectionDefaultsFilesProfilesAndWithdrawal$'
 if($LASTEXITCODE -ne 0){throw 'Initial Projects release-selected publication failed'}
 docker --context $DockerContext compose --profile dev --profile e03-regression restart postgres seaweed temporal nats owner-router storage-api storage-worker e03-storage-api e03-storage-worker projects-api projects-reconciler projects-event-relay generation-api generation-workflow generation-dispatcher generation-reconciler generation-controller generation-janitor generation-event-relay dev-consumer
 if($LASTEXITCODE -ne 0){throw 'Stand restart failed'}
 foreach($url in @('http://127.0.0.1:58094/readyz','http://127.0.0.1:58091/readyz','http://127.0.0.1:58090/readyz','http://127.0.0.1:58096/readyz')){
  $ready=$false
  for($attempt=0;$attempt -lt 60;$attempt++){
   try { $response=Invoke-WebRequest $url -TimeoutSec 2 -SkipHttpErrorCheck; if($response.StatusCode -eq 204 -or $response.StatusCode -eq 200){$ready=$true;break} } catch {}
   Start-Sleep -Seconds 1
  }
  if(-not $ready){throw "Service did not recover after restart: $url"}
 }
 $env:LEAFRUN_E03_RESTART_RECEIPT=$env:LEAFRUN_E03_RECEIPT
 Remove-Item Env:LEAFRUN_E03_RECEIPT
 go test -count=1 -v ./e03 -run '^TestRestartPersistence$'
 if($LASTEXITCODE -ne 0){throw 'Persistent receipt/object check failed'}
 $env:LEAFRUN_E04_RESTART_RECEIPT=$env:LEAFRUN_E04_RECEIPT_FILE
 Remove-Item Env:LEAFRUN_E04_RECEIPT_FILE
 go test -count=1 -v ./e04 -run '^TestLiveRestartPersistence$'
 if($LASTEXITCODE -ne 0){throw 'Persistent Generation receipt/PDF/event check failed'}
 $env:LEAFRUN_E05_RESTART_RECEIPT=$env:LEAFRUN_E05_RECEIPT_FILE
 Remove-Item Env:LEAFRUN_E05_RECEIPT_FILE
 go test -count=1 -v ./e04 -run '^TestE05RestartPersistence$'
 if($LASTEXITCODE -ne 0){throw 'Persistent Projects release/selection/prepared-input/PDF check failed'}
 docker --context $DockerContext compose --profile dev run --rm temporal-namespace
 if($LASTEXITCODE -ne 0){throw 'Persistent Temporal namespace unavailable'}
} finally {Remove-Item Env:LEAFRUN_E03_RECEIPT,Env:LEAFRUN_E03_RESTART_RECEIPT,Env:LEAFRUN_E04_RECEIPT_FILE,Env:LEAFRUN_E04_RESTART_RECEIPT,Env:LEAFRUN_E05_RECEIPT_FILE,Env:LEAFRUN_E05_RESTART_RECEIPT -ErrorAction SilentlyContinue;Pop-Location}
