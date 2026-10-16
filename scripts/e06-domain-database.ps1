param([string]$DockerContext='desktop-linux')
. (Join-Path $PSScriptRoot 'e06-common.ps1') -DockerContext $DockerContext
$e06DomainSecrets=Read-E06Secrets
$e06DomainPostgres=Get-E06Container postgres
foreach($e06Domain in @('generation','projects','storage','temporal','audit')) {
 $e06DomainPassword=$e06DomainSecrets["E06_$($e06Domain.ToUpperInvariant())_DB_PASSWORD"]
 if($e06DomainPassword -notmatch '^[0-9a-f]{64}$'){throw 'Domain database credential missing or invalid'}
 $e06DomainSQL=@'
SELECT format('CREATE ROLE %I LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS PASSWORD %L', :'domain', :'password') WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname=:'domain')
\gexec
SELECT format('CREATE DATABASE %I OWNER %I', :'domain', :'domain') WHERE NOT EXISTS (SELECT 1 FROM pg_database WHERE datname=:'domain')
\gexec
SELECT format('CREATE DATABASE %I OWNER %I', :'test_database', :'domain') WHERE NOT EXISTS (SELECT 1 FROM pg_database WHERE datname=:'test_database')
\gexec
SELECT format('REVOKE ALL ON DATABASE %I, %I FROM PUBLIC', :'domain', :'test_database')
\gexec
SELECT format('GRANT CONNECT ON DATABASE %I, %I TO %I', :'domain', :'test_database', :'domain')
\gexec
SELECT CASE WHEN EXISTS(SELECT 1 FROM pg_roles WHERE rolname=:'domain' AND (rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication OR rolbypassrls OR NOT rolcanlogin)) OR EXISTS(SELECT 1 FROM pg_auth_members m JOIN pg_roles r ON r.oid=m.member WHERE r.rolname=:'domain') OR EXISTS(SELECT 1 FROM pg_database d JOIN pg_roles r ON r.oid=d.datdba WHERE d.datname IN (:'domain',:'test_database') AND r.rolname<>:'domain') THEN 'invalid-owner' ELSE 'valid-owner' END;
'@
 $e06DomainSecondDatabase=if($e06Domain -eq 'temporal'){'temporal_visibility'}else{"${e06Domain}_test"}
 $e06DomainResult=$e06DomainSQL | & docker --context $DockerContext exec -i $e06DomainPostgres psql --username postgres --dbname postgres -v ON_ERROR_STOP=1 -v "domain=$e06Domain" -v "test_database=$e06DomainSecondDatabase" -v "password=$e06DomainPassword" 2>$null
 if($LASTEXITCODE -ne 0 -or ($e06DomainResult -join ' ') -match 'invalid-owner'){throw 'Owned domain database provisioning/owner validation failed; no reset attempted'}
 & docker --context $DockerContext exec -e "PGPASSWORD=$e06DomainPassword" $e06DomainPostgres psql -h 127.0.0.1 --username $e06Domain --dbname $e06DomainSecondDatabase -c 'SELECT 1' 2>$null
 if($LASTEXITCODE -ne 0 -or ($e06DomainResult -join ' ') -match 'invalid-owner'){throw 'Owned domain database credential mismatch; no reset attempted'}
}
Remove-Variable e06DomainSecrets,e06DomainPassword,e06DomainSQL -ErrorAction SilentlyContinue
& (Join-Path $PSScriptRoot 'e06-database-isolation.ps1') -DockerContext $DockerContext
Write-Output 'E06 owner domain databases ready; existing volumes and credentials preserved'
