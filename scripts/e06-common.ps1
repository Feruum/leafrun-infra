param([string]$DockerContext='desktop-linux')
$ErrorActionPreference='Stop'
$e06InfraRoot=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$e06TaskRoot=[IO.Path]::GetFullPath((Join-Path $e06InfraRoot '..'))
$e06EnvFile=Join-Path $e06InfraRoot '.env.e06'
$e06ComposeFile=Join-Path $e06InfraRoot 'compose.e06.yaml'
function Read-E06Secrets {
 if($IsWindows -and [Security.Principal.WindowsIdentity]::GetCurrent().Name -match 'CodexSandbox'){throw 'Private E06 runtime requires the normal user context'}
 $result=@{}
 foreach($line in Get-Content -LiteralPath $e06EnvFile){if(-not $line -or $line.StartsWith('#')){continue};$key,$value=$line -split '=',2;if($key -notmatch '^[A-Z][A-Z0-9_]*$' -or -not $value -or $result.ContainsKey($key)){throw 'Invalid private E06 runtime configuration'};$result[$key]=$value}
 return $result
}
function Invoke-E06Compose {
 param([Parameter(ValueFromRemainingArguments=$true)][string[]]$ComposeArguments)
 & docker --context $DockerContext compose --project-name leafrun-e06 --env-file $e06EnvFile -f $e06ComposeFile @ComposeArguments
 if($LASTEXITCODE -ne 0){throw 'E06 Compose operation failed'}
}
function Get-E06Container {
 param([Parameter(Mandatory=$true)][string]$Service)
 $id=Invoke-E06Compose ps -q $Service
 if(-not $id -or @($id).Count -ne 1){throw 'Owned E06 container unavailable'}
 $project=& docker --context $DockerContext inspect --format '{{index .Config.Labels "com.docker.compose.project"}}' $id
 if($LASTEXITCODE -ne 0 -or $project -ne 'leafrun-e06'){throw 'Refusing operation outside owned E06 project'}
 $actualService=& docker --context $DockerContext inspect --format '{{index .Config.Labels "com.docker.compose.service"}}' $id
 if($LASTEXITCODE -ne 0 -or $actualService -ne $Service){throw 'E06 container service mismatch'}
 return $id
}
function Set-E06PrivateFile {
 param([string]$Path)
 if($IsWindows){
  if([Security.Principal.WindowsIdentity]::GetCurrent().Name -match 'CodexSandbox'){throw 'Private E06 runtime requires the normal user context'}
  $sid=[Security.Principal.WindowsIdentity]::GetCurrent().User.Value
  $allowedSIDs=@($sid,'S-1-5-18')
  $grants=$allowedSIDs|ForEach-Object{"*${_}:(F)"}
  & icacls $Path /inheritance:r /grant:r @grants | Out-Null
  if($LASTEXITCODE -ne 0){throw 'Could not restrict E06 private file permissions'}
  & icacls $Path /setowner "*$sid" | Out-Null
  if($LASTEXITCODE -ne 0){throw 'Could not set E06 private file owner'}
  foreach($rule in (Get-Acl -LiteralPath $Path).Access){$ruleSID=$rule.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value;if($ruleSID -notin $allowedSIDs){& icacls $Path /remove:g "*$ruleSID" | Out-Null;if($LASTEXITCODE -ne 0){throw 'Could not remove broad E06 file permission'}}}
 }else{& chmod 600 -- $Path;if($LASTEXITCODE -ne 0){throw 'Could not restrict E06 private file permissions'}}
}
function Write-E06PrivateText {
 param([Parameter(Mandatory=$true)][string]$Path,[Parameter(Mandatory=$true)][AllowEmptyString()][string]$Text)
 $e06PrivateTarget=[IO.Path]::GetFullPath($Path)
 if(-not $e06PrivateTarget.StartsWith($e06InfraRoot+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)){throw 'Private E06 file path escapes Infra'}
 $e06PrivateTemporary=$e06PrivateTarget+'.'+[Guid]::NewGuid().ToString('N')+'.tmp'
 try {
  $e06PrivateStream=[IO.File]::Open($e06PrivateTemporary,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None)
  $e06PrivateStream.Dispose()
  Set-E06PrivateFile $e06PrivateTemporary
  [IO.File]::WriteAllText($e06PrivateTemporary,$Text,[Text.UTF8Encoding]::new($false))
  [IO.File]::Move($e06PrivateTemporary,$e06PrivateTarget,$true)
 }finally{if(Test-Path -LiteralPath $e06PrivateTemporary){Remove-Item -LiteralPath $e06PrivateTemporary -Force}}
}
