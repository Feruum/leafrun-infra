param([string]$DockerContext='desktop-linux')
. (Join-Path $PSScriptRoot 'e06-common.ps1') -DockerContext $DockerContext
$e06Values=Read-E06Secrets
foreach($key in @('E06_ACCESS_DB_PASSWORD','E06_KRATOS_DB_PASSWORD','E06_KETO_DB_PASSWORD','E06_HYDRA_DB_PASSWORD')){if($e06Values[$key] -notmatch '^[0-9a-f]{64}$'){throw 'Invalid E06 database credential format'}}
$e06Postgres=Get-E06Container postgres
$e06SQL=@'
SELECT format('CREATE ROLE access LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS PASSWORD %L', :'access_password') WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname='access')
\gexec
SELECT format('CREATE ROLE kratos LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS PASSWORD %L', :'kratos_password') WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname='kratos')
\gexec
SELECT format('CREATE ROLE keto LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS PASSWORD %L', :'keto_password') WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname='keto')
\gexec
SELECT format('CREATE ROLE hydra LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS PASSWORD %L', :'hydra_password') WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname='hydra')
\gexec
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='hydra' AND (NOT rolcanlogin OR rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication OR rolbypassrls)) OR EXISTS(SELECT 1 FROM pg_auth_members m JOIN pg_roles r ON r.oid=m.member WHERE r.rolname='hydra') THEN RAISE EXCEPTION 'Hydra role privileges mismatch'; END IF;
 IF EXISTS(SELECT 1 FROM pg_database d JOIN pg_roles r ON r.oid=d.datdba WHERE d.datname='hydra' AND r.rolname<>'hydra') THEN RAISE EXCEPTION 'Hydra database owner mismatch'; END IF;
END $$;
SELECT 'CREATE DATABASE hydra OWNER hydra' WHERE NOT EXISTS (SELECT 1 FROM pg_database WHERE datname='hydra')
\gexec
REVOKE ALL ON DATABASE hydra FROM PUBLIC;
GRANT CONNECT ON DATABASE hydra TO hydra;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname IN ('access','kratos','keto') AND (NOT rolcanlogin OR rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication OR rolbypassrls)) OR EXISTS(SELECT 1 FROM pg_auth_members m JOIN pg_roles r ON r.oid=m.member WHERE r.rolname IN ('access','kratos','keto')) THEN RAISE EXCEPTION 'Domain role privileges mismatch'; END IF;
 IF EXISTS(SELECT 1 FROM pg_database d JOIN pg_roles r ON r.oid=d.datdba WHERE (d.datname IN ('access','access_test') AND r.rolname<>'access') OR (d.datname='kratos' AND r.rolname<>'kratos') OR (d.datname='keto' AND r.rolname<>'keto')) THEN RAISE EXCEPTION 'Domain database owner mismatch'; END IF;
END $$;
SELECT 'CREATE DATABASE access OWNER access' WHERE NOT EXISTS (SELECT 1 FROM pg_database WHERE datname='access')
\gexec
SELECT 'CREATE DATABASE access_test OWNER access' WHERE NOT EXISTS (SELECT 1 FROM pg_database WHERE datname='access_test')
\gexec
SELECT 'CREATE DATABASE kratos OWNER kratos' WHERE NOT EXISTS (SELECT 1 FROM pg_database WHERE datname='kratos')
\gexec
SELECT 'CREATE DATABASE keto OWNER keto' WHERE NOT EXISTS (SELECT 1 FROM pg_database WHERE datname='keto')
\gexec
REVOKE ALL ON DATABASE access,access_test,kratos,keto,postgres,template1 FROM PUBLIC;
GRANT CONNECT ON DATABASE keto TO keto;
GRANT CONNECT ON DATABASE access,access_test TO access;
GRANT CONNECT ON DATABASE kratos TO kratos;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM (VALUES('access','kratos'),('access','keto'),('kratos','access'),('kratos','access_test'),('kratos','keto'),('keto','access'),('keto','access_test'),('keto','kratos')) p(role_name,database_name) WHERE has_database_privilege(p.role_name,p.database_name,'CONNECT')) THEN RAISE EXCEPTION 'Cross-domain database permission'; END IF;
END $$;
'@
$e06SQL | & docker --context $DockerContext exec -i $e06Postgres psql --username postgres --dbname postgres -v ON_ERROR_STOP=1 -v "access_password=$($e06Values.E06_ACCESS_DB_PASSWORD)" -v "kratos_password=$($e06Values.E06_KRATOS_DB_PASSWORD)" -v "keto_password=$($e06Values.E06_KETO_DB_PASSWORD)" -v "hydra_password=$($e06Values.E06_HYDRA_DB_PASSWORD)" 2>$null | Out-Null
if($LASTEXITCODE -ne 0){throw 'Owned E06 database provisioning or ownership validation failed'}
foreach($test in @(@{role='access';database='access';password=$e06Values.E06_ACCESS_DB_PASSWORD},@{role='access';database='access_test';password=$e06Values.E06_ACCESS_DB_PASSWORD},@{role='kratos';database='kratos';password=$e06Values.E06_KRATOS_DB_PASSWORD},@{role='keto';database='keto';password=$e06Values.E06_KETO_DB_PASSWORD})){
 & docker --context $DockerContext exec -e "PGPASSWORD=$($test.password)" $e06Postgres psql -h 127.0.0.1 --username $test.role --dbname $test.database -v ON_ERROR_STOP=1 -c 'SELECT 1' 2>$null | Out-Null
 if($LASTEXITCODE -ne 0){throw 'Existing owned database credential mismatch; refusing reset'}
}
foreach($test in @(@{role='access';database='kratos';password=$e06Values.E06_ACCESS_DB_PASSWORD},@{role='access';database='keto';password=$e06Values.E06_ACCESS_DB_PASSWORD},@{role='kratos';database='access';password=$e06Values.E06_KRATOS_DB_PASSWORD},@{role='kratos';database='access_test';password=$e06Values.E06_KRATOS_DB_PASSWORD},@{role='kratos';database='keto';password=$e06Values.E06_KRATOS_DB_PASSWORD},@{role='keto';database='access';password=$e06Values.E06_KETO_DB_PASSWORD},@{role='keto';database='access_test';password=$e06Values.E06_KETO_DB_PASSWORD},@{role='keto';database='kratos';password=$e06Values.E06_KETO_DB_PASSWORD})){
 & docker --context $DockerContext exec -e "PGPASSWORD=$($test.password)" $e06Postgres psql -h 127.0.0.1 --username $test.role --dbname $test.database -c 'SELECT 1' 2>$null | Out-Null
 if($LASTEXITCODE -eq 0){throw 'Cross-domain database connection unexpectedly permitted'}
}
Remove-Variable e06Values,e06SQL,test -ErrorAction SilentlyContinue
& (Join-Path $PSScriptRoot 'e06-database-isolation.ps1') -DockerContext $DockerContext
Write-Output 'E06 database ownership, restricted roles, credentials and cross-domain denial verified'
