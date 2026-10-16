param([Parameter(Mandatory=$true)][string]$TypstBinary,[string]$DockerContext='desktop-linux',[switch]$Start,[switch]$VerifyPins,[string]$DependencyManifest='')
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'e06-common.ps1') -DockerContext $DockerContext
. (Join-Path $PSScriptRoot 'workspace.ps1') -FullE06
if(-not $DependencyManifest){$DependencyManifest=Join-Path $e06InfraRoot 'ci/dependencies.json'}
$pins=Get-Content -LiteralPath $DependencyManifest -Raw|ConvertFrom-Json
$run=Join-Path $e06InfraRoot ('.cache/e06/qualification-'+[Guid]::NewGuid().ToString('N'))
[IO.Directory]::CreateDirectory($run)|Out-Null
$receipt=[ordered]@{schemaVersion=1;startedAt=[DateTime]::UtcNow.ToString('o');completedAt=$null;passed=$false;inputs=[ordered]@{};modules=@();tests=@{passed=0;failed=0;skipped=0;skipNames=@()};flags=@{linuxRace=$true;ownerIntegration=$true;auditIntegration=$true;browserAutomation=$false;exclusiveAuditConsumer=$false};limitations=@('Browser automation and destructive outage coordination have separate assertion receipts.','E03/E04/E05 dev-auth regression is a separate run.');toolchain=@{}}
$rawLog=Join-Path $run 'private-tests.jsonl'
Write-E06PrivateText -Path $rawLog -Text ''
$previousExportRequirement=$env:LEAFRUN_SOURCE_REQUIRE_CLEAN
if($VerifyPins){$env:LEAFRUN_SOURCE_REQUIRE_CLEAN='1'}
try {
 foreach($name in @('contracts','platform','access','audit','core','integrations','storage','projects','generation','infra')){
  $source=$paths[$name]
  $safe='safe.directory='+$source.Replace('\','/')
  $commit=& git -c $safe -C $source rev-parse HEAD
  if($LASTEXITCODE -ne 0){throw "Cannot identify $name source"}
  $tree=& git -c $safe -C $source rev-parse ($commit+'^{tree}')
  if($LASTEXITCODE -ne 0){throw "Cannot identify $name tree"}
  $dirty=@(& git -c $safe -C $source status --porcelain)
  if($LASTEXITCODE -ne 0){throw "Cannot inspect $name source"}
  if($VerifyPins){
   if($dirty.Count){throw "Pinned E06 verification requires clean $name source"}
   if($name -ne 'infra'){
    $pin=$pins.$name
    if($pin.ref -notmatch '^[a-f0-9]{40}$' -or $pin.tree -notmatch '^[a-f0-9]{40}$' -or $pin.repository -notmatch '^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$' -or $pin.ref -ne $commit -or $pin.tree -ne $tree){throw "E06 dependency ref/tree mismatch: $name"}
   }
  }
  $files=@(& git -c $safe -C $source ls-files)
  if($LASTEXITCODE -ne 0){throw "Cannot enumerate $name input"}
  $hashes=@(foreach($file in $files){
   if($file -match '(^|/)(\.cache|\.git|\.aws|\.ssh|\.codex|\.agents)(/|$)' -or $file -match '(^|/)\.env(?:\.[^/]*)?$'){throw 'Private path in E06 tracked input'}
   @{path=$file;sha256=(Get-FileHash -LiteralPath (Join-Path $source $file) -Algorithm SHA256).Hash.ToLowerInvariant()}
  })
  $receipt.inputs[$name]=@{commit=$commit;tree=$tree;dirty=($dirty.Count -gt 0);files=$hashes}
 }
 $receipt.toolchain.docker=& docker --context $DockerContext version --format '{{.Client.Version}}/{{.Server.Version}}'
 if($LASTEXITCODE -ne 0){throw 'Docker version probe failed'}
 $receipt.toolchain.compose=& docker --context $DockerContext compose version --short
 if($LASTEXITCODE -ne 0){throw 'Compose version probe failed'}
 $receipt.toolchain.buildx=& docker --context $DockerContext buildx version
 if($LASTEXITCODE -ne 0){throw 'Buildx version probe failed'}
 $receipt.toolchain.typstSHA256=(Get-FileHash -LiteralPath $TypstBinary -Algorithm SHA256).Hash.ToLowerInvariant()
 if($Start){
  & (Join-Path $PSScriptRoot 'e06-init-env.ps1') -DockerContext $DockerContext
  if($env:GITHUB_ACTIONS -eq 'true'){foreach($secretLine in Get-Content -LiteralPath $e06EnvFile){$secretName,$secretValue=$secretLine -split '=',2;Write-Output "::add-mask::$secretValue"};Remove-Variable secretLine,secretName,secretValue}
  & (Join-Path $PSScriptRoot 'e06-start.ps1') -DockerContext $DockerContext
  & (Join-Path $PSScriptRoot 'e06-owners-start.ps1') -DockerContext $DockerContext -Build
 }
 foreach($service in @('access-api-a','access-api-b','generation-api','projects-api','storage-api','audit-api','audit-consumer','generation-workflow','temporal','seaweed')){Get-E06Container $service|Out-Null}
 Invoke-E06Compose --profile checks --profile replicas config --quiet
 Invoke-E06Compose --profile checks --profile replicas up --detach --no-deps kratos-expiry hydra-expiry generation-api-replica projects-api-replica storage-api-replica
 $qualificationFailure=$null
 try {& (Join-Path $PSScriptRoot 'e06-owner-check.ps1') -DockerContext $DockerContext -TypstBinary $TypstBinary -AllModules -Sandbox -TestContainerName ('leafrun-e06-qualification-'+[Guid]::NewGuid().ToString('N')) *> $rawLog} catch {$qualificationFailure=$true}
 $events=@(foreach($line in [IO.File]::ReadLines($rawLog)){if($line.StartsWith('{')){try{ConvertFrom-Json -InputObject $line -ErrorAction Stop}catch{}}})
 foreach($event in $events){
  if($event.qualificationModule){$receipt.modules+=@{repository=$event.qualificationModule;raceVetBuild=$event.raceVetBuild}}
  if($event.Test -and $event.Action -in @('pass','fail','skip')){
   switch($event.Action){
    pass {$receipt.tests.passed++}
    fail {$receipt.tests.failed++}
    skip {$receipt.tests.skipped++;$receipt.tests.skipNames+=@{package=$event.Package;test=$event.Test}}
   }
  }
 }
 if($qualificationFailure -or $receipt.modules.Count -ne 10 -or $receipt.tests.failed -ne 0 -or $receipt.tests.passed -eq 0){throw 'E06 qualification did not complete all ten modules without failures'}
 foreach($name in $receipt.inputs.Keys){
  $source=$paths[$name]
  $after=& git -c ('safe.directory='+$source.Replace('\','/')) -C $source rev-parse HEAD
  if($LASTEXITCODE -ne 0 -or $after -ne $receipt.inputs[$name].commit){throw 'Selected source HEAD changed during qualification'}
  foreach($file in $receipt.inputs[$name].files){if((Get-FileHash -LiteralPath (Join-Path $source $file.path) -Algorithm SHA256).Hash.ToLowerInvariant() -ne $file.sha256){throw 'Selected working input changed during qualification'}}
 }
 $receipt.runtimeImages=@{}
 foreach($service in @('access-api-a','access-api-b','generation-api','generation-api-replica','storage-api','storage-api-replica','projects-api','projects-api-replica','audit-api','audit-consumer','owner-router')){
  $container=Get-E06Container $service
  $image=& docker --context $DockerContext inspect --format '{{.Image}}' $container
  if($LASTEXITCODE -ne 0 -or $image -notmatch '^sha256:[0-9a-f]{64}$'){throw 'Runtime image identity unavailable'}
  $receipt.runtimeImages[$service]=$image
 }
 $receipt.flags.rendererSandbox=$true
 $exportFile=Join-Path $e06InfraRoot '.cache/source-export-evidence.json'
 if($Start){
  $export=Get-Content -LiteralPath $exportFile -Raw|ConvertFrom-Json
  foreach($name in $receipt.inputs.Keys){if($export.repositories.$name.commit -ne $receipt.inputs[$name].commit -or $export.repositories.$name.tree -ne $receipt.inputs[$name].tree){throw 'Runtime build export does not match selected source'}}
  $receipt.sourceExportSHA256=(Get-FileHash -LiteralPath $exportFile -Algorithm SHA256).Hash.ToLowerInvariant()
 }
 $receipt.passed=$true
} finally {
 if($null -eq $previousExportRequirement){Remove-Item Env:LEAFRUN_SOURCE_REQUIRE_CLEAN -ErrorAction SilentlyContinue}else{$env:LEAFRUN_SOURCE_REQUIRE_CLEAN=$previousExportRequirement}
 $receipt.completedAt=[DateTime]::UtcNow.ToString('o')
 [IO.File]::WriteAllText((Join-Path $run 'receipt.json'),($receipt|ConvertTo-Json -Depth 12))
 Write-Output "Sanitized E06 qualification receipt: $run/receipt.json"
}
