param([string]$IntegrationsDirectory=$env:LEAFRUN_INTEGRATIONS_DIR,[switch]$FullE06)
$ErrorActionPreference='Stop'
$infraRoot=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$taskRoot=[IO.Path]::GetFullPath((Join-Path $infraRoot '..'))
if(-not $IntegrationsDirectory){foreach($candidate in @((Join-Path $taskRoot 'integrations'),(Join-Path $taskRoot '../integrations'))){if(Test-Path -LiteralPath (Join-Path $candidate 'go.mod')){$IntegrationsDirectory=[IO.Path]::GetFullPath($candidate);break}}}
if(-not $IntegrationsDirectory){throw 'Set LEAFRUN_INTEGRATIONS_DIR to the M02 integrations checkout'}
$paths=@{core=(Join-Path $taskRoot 'core');contracts=(Join-Path $taskRoot 'contracts');storage=(Join-Path $taskRoot 'storage');generation=(Join-Path $taskRoot 'generation');projects=(Join-Path $taskRoot 'projects');platform=(Join-Path $taskRoot 'platform');infra=$infraRoot;integrations=[IO.Path]::GetFullPath($IntegrationsDirectory)}
foreach($optionalModule in @('access','audit')){ $optionalPath=Join-Path $taskRoot $optionalModule; if(Test-Path -LiteralPath (Join-Path $optionalPath 'go.mod')){$paths[$optionalModule]=$optionalPath} }
if($FullE06){foreach($requiredModule in @('access','audit')){if(-not $paths.ContainsKey($requiredModule)){throw "E06 requires the $requiredModule checkout"}}}
foreach($path in $paths.Values){if(-not(Test-Path -LiteralPath (Join-Path $path 'go.mod'))){throw "Missing module checkout: $path"}}
$runtime=Join-Path $infraRoot '.cache'
[IO.Directory]::CreateDirectory($runtime)|Out-Null
$lines=@('go 1.27.0','use (')
foreach($path in $paths.Values){$lines+=' '+ '"'+[IO.Path]::GetRelativePath($runtime,$path).Replace('\','/')+'"'}
$lines+=')'
foreach($name in @('contracts','document-core','integrations','platform')){$key=if($name -eq 'document-core'){'core'}else{$name};$lines+='replace github.com/leafrun/leafrun-'+$name+' v0.1.0 => "'+[IO.Path]::GetRelativePath($runtime,$paths[$key]).Replace('\','/')+'"'}
$env:GOWORK=Join-Path $runtime 'e05.work'
[IO.File]::WriteAllText($env:GOWORK,($lines -join "`n")+"`n")
