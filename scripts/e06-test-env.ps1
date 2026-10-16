# Dot-source: private credentials are loaded without output.
param([string]$DockerContext='desktop-linux')
. (Join-Path $PSScriptRoot 'e06-common.ps1') -DockerContext $DockerContext
$e06Values=Read-E06Secrets
$env:ACCESS_TEST_DATABASE_URL="postgres://access:$($e06Values.E06_ACCESS_DB_PASSWORD)@127.0.0.1:58534/access_test?sslmode=disable"
foreach($e06TestDomain in @('generation','projects','storage')) {
 $e06TestPassword=$e06Values["E06_$($e06TestDomain.ToUpperInvariant())_DB_PASSWORD"]
 if($e06TestPassword -notmatch '^[0-9a-f]{64}$'){throw 'Missing owner test database credential'}
 [Environment]::SetEnvironmentVariable("$($e06TestDomain.ToUpperInvariant())_TEST_DATABASE_URL","postgres://${e06TestDomain}:$e06TestPassword@127.0.0.1:58534/${e06TestDomain}_test?sslmode=disable",'Process')
}
. (Join-Path $PSScriptRoot 'workspace.ps1') -FullE06
$e06WorkspaceRoot=[IO.Path]::GetFullPath((Join-Path $e06TaskRoot '../../..'))
$env:GOCACHE=Join-Path $e06WorkspaceRoot '.cache/go-build'
$env:GOMODCACHE=Join-Path $e06WorkspaceRoot '.cache/go-mod'
Remove-Variable e06Values -ErrorAction SilentlyContinue
Remove-Variable e06TestDomain,e06TestPassword -ErrorAction SilentlyContinue
