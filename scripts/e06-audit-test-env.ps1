# Dot-source in the normal user context; no credential values are emitted.
param([string]$DockerContext='desktop-linux')
. (Join-Path $PSScriptRoot 'e06-common.ps1') -DockerContext $DockerContext
$e06AuditValues=Read-E06Secrets
if($e06AuditValues.E06_AUDIT_DB_PASSWORD -notmatch '^[0-9a-f]{64}$'){throw 'Owned Audit test credential unavailable'}
$env:LEAFRUN_AUDIT_TEST_DATABASE_URL="postgres://audit:$($e06AuditValues.E06_AUDIT_DB_PASSWORD)@127.0.0.1:58534/audit_test?sslmode=disable"
$env:E06_AUDIT_NATS_URL='nats://127.0.0.1:54322'
foreach($e06AuditRole in @('access','generation','projects','storage','audit','admin')){
 $e06AuditKey='E06_NATS_'+$e06AuditRole.ToUpperInvariant()+'_PASSWORD'
 if($e06AuditValues[$e06AuditKey] -notmatch '^[0-9a-f]{64}$'){throw 'Owned Audit broker test credential unavailable'}
 [Environment]::SetEnvironmentVariable('E06_AUDIT_NATS_'+$e06AuditRole.ToUpperInvariant()+'_PASSWORD',$e06AuditValues[$e06AuditKey],'Process')
}
Remove-Variable e06AuditValues,e06AuditRole,e06AuditKey -ErrorAction SilentlyContinue
