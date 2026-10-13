# Export tracked source files only. Credentials, Git metadata and local caches never enter builds.
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'workspace.ps1')
$contextRoot=[IO.Path]::GetFullPath((Join-Path $infraRoot '.cache/context'))
[IO.Directory]::CreateDirectory($contextRoot)|Out-Null
foreach($repo in @('core','contracts','storage','integrations','infra')){
 $source=$paths[$repo]
 $safe='safe.directory='+$source.Replace('\','/')
 $files=git -c $safe -C $source ls-files
 if($LASTEXITCODE -ne 0){throw "Cannot export $repo"}
 $target=[IO.Path]::GetFullPath((Join-Path $contextRoot $repo))
 if(-not $target.StartsWith($contextRoot+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)){throw 'Context path escapes cache'}
 if(Test-Path -LiteralPath $target){Remove-Item -LiteralPath $target -Recurse -Force}
 [IO.Directory]::CreateDirectory($target)|Out-Null
 foreach($file in $files){
  if($file -match '(^|/)(\.env|\.cache|\.git)(/|$)'){throw 'Private path in tracked source'}
  $destination=[IO.Path]::GetFullPath((Join-Path $target $file))
  if(-not $destination.StartsWith($target+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)){throw 'Tracked path escapes context'}
  [IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($destination))|Out-Null
  Copy-Item -LiteralPath (Join-Path $source $file) -Destination $destination
 }
}
Write-Output 'Tracked source build contexts prepared'
