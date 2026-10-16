param([string]$DockerContext='desktop-linux')
. (Join-Path $PSScriptRoot 'e06-common.ps1') -DockerContext $DockerContext
$e06IsolationValues=Read-E06Secrets
$e06IsolationPostgres=Get-E06Container postgres
# Probe only existing owned catalogs so provider-only startup also works.
$e06Catalogs=& docker --context $DockerContext exec $e06IsolationPostgres psql --username postgres --dbname postgres --tuples-only --no-align -v ON_ERROR_STOP=1 -c "SELECT datname FROM pg_database WHERE datname IN ('access','access_test','kratos','keto','hydra','generation','generation_test','projects','projects_test','storage','storage_test','temporal','temporal_visibility','audit','audit_test') ORDER BY datname" 2>$null
if($LASTEXITCODE -ne 0 -or 'hydra' -notin $e06Catalogs){throw 'Owned isolation catalog discovery failed'}
$e06IsolationRoles=@('access','kratos','keto','hydra','generation','projects','storage','temporal','audit')
foreach($e06Role in $e06IsolationRoles){
 if($e06Role -notin $e06Catalogs){continue}
 $e06Password=$e06IsolationValues["E06_$($e06Role.ToUpperInvariant())_DB_PASSWORD"]
 if($e06Password -notmatch '^[0-9a-f]{64}$'){throw 'Owned isolation credential missing'}
 foreach($e06Catalog in $e06Catalogs){
  $e06Owns=($e06Catalog -eq $e06Role -or $e06Catalog -eq "${e06Role}_test" -or ($e06Role -eq 'temporal' -and $e06Catalog -eq 'temporal_visibility'))
  & docker --context $DockerContext exec -e "PGPASSWORD=$e06Password" $e06IsolationPostgres psql -h 127.0.0.1 --username $e06Role --dbname $e06Catalog -v ON_ERROR_STOP=1 -c 'SELECT 1' 2>$null | Out-Null
  if(($LASTEXITCODE -eq 0) -ne $e06Owns){throw 'Owned database matching credential or pairwise isolation failed; refusing reset'}
 }
}
Remove-Variable e06IsolationValues,e06Password,e06Catalogs -ErrorAction SilentlyContinue
Write-Output 'All existing E06 database roles have matching credentials and pairwise cross-domain connection denial'
