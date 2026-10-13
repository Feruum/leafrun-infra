param([Parameter(Mandatory=$true)][string]$TypstBinary,[string]$DockerContext='desktop-linux',[switch]$VerifyPins)
$ErrorActionPreference='Stop'
Push-Location (Join-Path $PSScriptRoot '..')
try {
 $infraRoot=[IO.Path]::GetFullPath((Get-Location).Path)
 $taskRoot=[IO.Path]::GetFullPath((Join-Path $infraRoot '..'))
 $evidence=[ordered]@{schemaVersion=1;startedAt=[DateTime]::UtcNow.ToString('o');completedAt=$null;inputs=[ordered]@{};checks=[ordered]@{};passed=$false}
 $evidenceFile=Join-Path $infraRoot '.cache/ci-evidence.json'
 [IO.Directory]::CreateDirectory((Split-Path $evidenceFile))|Out-Null
 $pins=Get-Content -Raw -LiteralPath 'ci/dependencies.json'|ConvertFrom-Json
 foreach($repo in @('infra','core','contracts','storage','integrations')){
  $source=Join-Path $taskRoot $repo
  if($repo -eq 'integrations' -and -not(Test-Path -LiteralPath (Join-Path $source 'go.mod'))){$source=[IO.Path]::GetFullPath((Join-Path $taskRoot '../integrations'))}
  if($repo -eq 'integrations' -and $env:LEAFRUN_INTEGRATIONS_DIR){$source=[IO.Path]::GetFullPath($env:LEAFRUN_INTEGRATIONS_DIR)}
  $commit=git -c "safe.directory=$($source.Replace('\','/'))" -C $source rev-parse HEAD
  if($LASTEXITCODE -ne 0){throw "Cannot identify $repo source"}
  $tree=git -c "safe.directory=$($source.Replace('\','/'))" -C $source rev-parse 'HEAD^{tree}'
  if($LASTEXITCODE -ne 0){throw "Cannot identify $repo tree"}
  $changes=git -c "safe.directory=$($source.Replace('\','/'))" -C $source status --porcelain
  if($LASTEXITCODE -ne 0){throw "Cannot inspect $repo checkout"}
  if($VerifyPins -and $changes){throw "Pinned verification requires a clean $repo checkout"}
  if($VerifyPins -and $repo -ne 'infra' -and $commit -ne $pins.$repo.ref){throw "Checkout does not match pinned $repo dependency"}
  $evidence.inputs[$repo]=[ordered]@{commit=$commit;tree=$tree;dirty=[bool]$changes}
 }
 & ./scripts/init-env.ps1
 # Runner log masking is registered before any service or test can emit errors.
 if($env:GITHUB_ACTIONS -eq 'true'){Get-Content -LiteralPath .env|ForEach-Object{$key,$value=$_ -split '=',2;Write-Output "::add-mask::$value"}}
 & ./scripts/start.ps1 -DockerContext $DockerContext
 $evidence.checks.standStart=$true
 . ./scripts/test-env.ps1
 $goVersion=& go version
 if($LASTEXITCODE -ne 0 -or $goVersion -notmatch '^go version go1\.27\.'){throw 'Go 1.27 must be on PATH'}
 $typstVersion=& typst --version
 if($LASTEXITCODE -ne 0 -or $typstVersion -notmatch '^typst 0\.15\.1\b'){throw 'Typst 0.15.1 must be on PATH'}
 $evidence.toolchain=[ordered]@{go=$goVersion;typst=$typstVersion}
 go test -count=1 ./...
 if($LASTEXITCODE -ne 0){throw 'Host integration tests failed'}
 $evidence.checks.hostIntegration=$true
 go vet ./...
 if($LASTEXITCODE -ne 0){throw 'Host vet failed'}
 $evidence.checks.hostVet=$true
 go build ./...
 if($LASTEXITCODE -ne 0){throw 'Host build failed'}
 $evidence.checks.hostBuild=$true
 & ./scripts/restart-check.ps1 -DockerContext $DockerContext
 $evidence.checks.restartPersistence=$true
 & ./scripts/linux-checks.ps1 -DockerContext $DockerContext -TypstBinary $TypstBinary
 $evidence.checks.allRepositoryLinuxRaceVetBuild=$true
 $evidence.passed=$true
} finally {
 if($null -ne $evidence){$evidence.completedAt=[DateTime]::UtcNow.ToString('o');[IO.File]::WriteAllText($evidenceFile,($evidence|ConvertTo-Json -Depth 8))}
 Pop-Location
}
