# Export tracked source files only. Credentials, Git metadata and local caches never enter builds.
param([switch]$RequireClean=($env:LEAFRUN_SOURCE_REQUIRE_CLEAN -eq '1'))
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'workspace.ps1')
$contextRoot=[IO.Path]::GetFullPath((Join-Path $infraRoot '.cache/context'))
[IO.Directory]::CreateDirectory($contextRoot)|Out-Null
$exportEvidence=@{at=[DateTime]::UtcNow.ToString('o');requireClean=[bool]$RequireClean;repositories=@{}}
foreach($repo in $paths.Keys){
 $source=$paths[$repo]
 $safe='safe.directory='+$source.Replace('\','/')
 $commit=git -c $safe -C $source rev-parse HEAD
 if($LASTEXITCODE -ne 0){throw "Cannot identify $repo export source"}
 $tree=git -c $safe -C $source rev-parse ($commit+'^{tree}')
 if($LASTEXITCODE -ne 0){throw "Cannot identify $repo export tree"}
 $dirty=@(git -c $safe -C $source status --porcelain)
 if($LASTEXITCODE -ne 0 -or ($RequireClean -and $dirty.Count)){throw "Export requires clean $repo source"}
 $exportFiles=@()
 $files=git -c $safe -C $source ls-files
 if($LASTEXITCODE -ne 0){throw "Cannot export $repo"}
 $target=[IO.Path]::GetFullPath((Join-Path $contextRoot $repo))
 if(-not $target.StartsWith($contextRoot+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)){throw 'Context path escapes cache'}
 if(Test-Path -LiteralPath $target){Remove-Item -LiteralPath $target -Recurse -Force}
 [IO.Directory]::CreateDirectory($target)|Out-Null
 foreach($file in $files){
  if($file -match '(^|/)(\.env(?:\.[^/]*)?|\.cache|\.git|\.aws|\.ssh|\.codex|\.agents)(/|$)'){throw 'Private path in tracked source'}
  $destination=[IO.Path]::GetFullPath((Join-Path $target $file))
  if(-not $destination.StartsWith($target+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)){throw 'Tracked path escapes context'}
  [IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($destination))|Out-Null
  Copy-Item -LiteralPath (Join-Path $source $file) -Destination $destination
  $sourceHash=(Get-FileHash -LiteralPath (Join-Path $source $file) -Algorithm SHA256).Hash.ToLowerInvariant()
  $exportHash=(Get-FileHash -LiteralPath $destination -Algorithm SHA256).Hash.ToLowerInvariant()
  if($sourceHash -ne $exportHash){throw 'Exported bytes differ from selected working source'}
  $exportFiles+=@{path=$file;sourceSHA256=$sourceHash;exportSHA256=$exportHash}
 }
 $afterCommit=git -c $safe -C $source rev-parse HEAD
 if($LASTEXITCODE -ne 0 -or $afterCommit -ne $commit){throw 'Source HEAD changed during export'}
 $exportEvidence.repositories[$repo]=@{commit=$commit;tree=$tree;dirty=($dirty.Count -gt 0);files=$exportFiles}
}
[IO.File]::WriteAllText((Join-Path $infraRoot '.cache/source-export-evidence.json'),($exportEvidence|ConvertTo-Json -Depth 10))
Write-Output 'Tracked source build contexts prepared'
