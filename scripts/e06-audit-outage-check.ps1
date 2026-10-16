param([string]$DockerContext='desktop-linux')
. (Join-Path $PSScriptRoot 'e06-common.ps1') -DockerContext $DockerContext
$e06OutageRoot=Join-Path $e06InfraRoot ('.cache/e06/audit-outage-'+[Guid]::NewGuid().ToString('N'))
[IO.Directory]::CreateDirectory($e06OutageRoot)|Out-Null
$e06OutageContainer='leafrun-e06-audit-outage-'+[Guid]::NewGuid().ToString('N')
$e06CheckScript=Join-Path $PSScriptRoot 'e06-owner-check.ps1'
$e06Runner=Join-Path $e06OutageRoot 'run.ps1'
$e06EscapedCheck=$e06CheckScript.Replace("'","''")
$e06EscapedDirectory=$e06OutageRoot.Replace("'","''")
$e06EscapedContext=$DockerContext.Replace("'","''")
$e06RunnerText="& '$e06EscapedCheck' -DockerContext '$e06EscapedContext' -AuditOutageDirectory '$e06EscapedDirectory' -TestContainerName '$e06OutageContainer' -GoArguments @('-json','./e06','-run','^TestRealAuditFourOwnerFactsAndCurrentReader$')`nif(-not `$?){exit 1}`n"
[IO.File]::WriteAllText($e06Runner,$e06RunnerText,[Text.UTF8Encoding]::new($false))
$e06Broker=Get-E06Container nats
$e06Consumer=Get-E06Container audit-consumer
$e06BrokerMounts=& docker --context $DockerContext inspect --format '{{range .Mounts}}{{if eq .Type "volume"}}{{.Name}}{{end}}{{end}}' $e06Broker
if($LASTEXITCODE -ne 0 -or $e06BrokerMounts -ne 'leafrun-e06_e06-jetstream-data'){throw 'Audit outage requires the owned persistent broker volume'}
$e06Process=$null
$e06BrokerStopped=$false
function Wait-E06AuditMarker {
 param([string]$Name,[int]$Seconds=180)
 $e06Deadline=[DateTime]::UtcNow.AddSeconds($Seconds)
 while([DateTime]::UtcNow -lt $e06Deadline){
  if(Test-Path -LiteralPath (Join-Path $e06OutageRoot $Name)){
   $e06Marker=Get-Content -LiteralPath (Join-Path $e06OutageRoot $Name) -Raw|ConvertFrom-Json
   if(-not $e06Marker.runId -or $e06Marker.runId.Length -gt 128){throw 'Invalid owned Audit outage handshake'}
   return $e06Marker
  }
  if($e06Process.HasExited){throw 'Owned Audit outage test exited before its handshake; retain logs'}
  Start-Sleep -Milliseconds 100
 }
 throw 'Owned Audit outage handshake timed out; retain logs'
}
try {
 $e06Process=Start-Process -FilePath (Get-Process -Id $PID).Path -ArgumentList @('-NoLogo','-NoProfile','-File',('"'+$e06Runner+'"')) -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $e06OutageRoot 'test.jsonl') -RedirectStandardError (Join-Path $e06OutageRoot 'test.stderr')
 $e06Ready=Wait-E06AuditMarker 'ready.json'
 Invoke-E06Compose stop nats
 $e06BrokerStopped=$true
 @{runId=$e06Ready.runId}|ConvertTo-Json|Set-Content (Join-Path $e06OutageRoot 'broker-down.json')
 $e06Committed=Wait-E06AuditMarker 'committed.json' 360
 if($e06Committed.runId -ne $e06Ready.runId){throw 'Audit outage run identity changed'}
 Invoke-E06Compose up --detach --wait --no-deps nats
 $e06BrokerStopped=$false
 Invoke-E06Compose restart --no-deps audit-consumer
 @{runId=$e06Ready.runId}|ConvertTo-Json|Set-Content (Join-Path $e06OutageRoot 'broker-restored.json')
 $e06ExitDeadline=[DateTime]::UtcNow.AddSeconds(360)
 while(-not $e06Process.HasExited -and [DateTime]::UtcNow -lt $e06ExitDeadline){Start-Sleep -Milliseconds 100}
 if(-not $e06Process.HasExited){throw 'Audit outage recovery test timed out; retain logs'}
 $e06Process.WaitForExit()
 if($e06Process.ExitCode -ne 0){throw 'Audit outage recovery test failed; retain logs'}
 $e06BrokerAfter=Get-E06Container nats
 $e06MountsAfter=& docker --context $DockerContext inspect --format '{{range .Mounts}}{{if eq .Type "volume"}}{{.Name}}{{end}}{{end}}' $e06BrokerAfter
 if($LASTEXITCODE -ne 0 -or $e06MountsAfter -ne $e06BrokerMounts){throw 'Audit outage broker volume identity changed'}
 @{at=[DateTime]::UtcNow.ToString('o');status='PASS';runId=$e06Ready.runId;brokerActuallyStopped=$true;consumerActuallyRestarted=$true;brokerVolumePreserved=$true;scope='Fresh four-owner committed actions, actual NATS outage/recovery, separate current-reader/cursor/privacy and bounded captured-fact transport assertions';testExit=$e06Process.ExitCode}|ConvertTo-Json|Set-Content (Join-Path $e06OutageRoot 'receipt.json')
 Write-Output "Owned E06 Audit outage/recovery passed; evidence: $e06OutageRoot"
} finally {
 if($e06BrokerStopped){Invoke-E06Compose up --detach --wait --no-deps nats}
 if($e06Process -and -not $e06Process.HasExited){
  # Stop only the uniquely named test container created by this invocation.
  $e06OwnedName=& docker --context $DockerContext inspect --format '{{.Name}}' $e06OutageContainer 2>$null
  if($LASTEXITCODE -eq 0 -and $e06OwnedName -eq "/$e06OutageContainer"){& docker --context $DockerContext stop $e06OutageContainer|Out-Null}
 }
}
