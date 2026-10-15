param([string]$DockerContext='desktop-linux')
$ErrorActionPreference='Stop'
$projectsInfra=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$projectsConfig=@{}
Get-Content -LiteralPath (Join-Path $projectsInfra '.env') | ForEach-Object { $key,$value=$_ -split '=',2; $projectsConfig[$key]=$value }
if($projectsConfig.PROJECTS_DB_PASSWORD -notmatch '^[0-9a-f]{64}$'){throw 'Projects database requires the generated private credential'}
$projectsPostgres=docker --context $DockerContext compose --env-file (Join-Path $projectsInfra '.env') -f (Join-Path $projectsInfra 'compose.yaml') --profile dev ps -q postgres
if($LASTEXITCODE -ne 0 -or -not $projectsPostgres){throw 'The internal PostgreSQL service is unavailable'}
# This also upgrades the already-owned E04 development volume without touching
# other domains or replacing an existing Projects role's credential.
$projectsSQL=@'
SELECT format('CREATE ROLE projects LOGIN PASSWORD %L', :'projects_password') WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname='projects')
\gexec
SELECT 'CREATE DATABASE projects OWNER projects' WHERE NOT EXISTS (SELECT 1 FROM pg_database WHERE datname='projects')
\gexec
SELECT 'CREATE DATABASE projects_test OWNER projects' WHERE NOT EXISTS (SELECT 1 FROM pg_database WHERE datname='projects_test')
\gexec
REVOKE CONNECT ON DATABASE projects, projects_test FROM PUBLIC;
GRANT CONNECT ON DATABASE projects, projects_test TO projects;
'@
$projectsSQL | docker --context $DockerContext exec -i $projectsPostgres psql --username postgres --dbname postgres -v ON_ERROR_STOP=1 -v "projects_password=$($projectsConfig.PROJECTS_DB_PASSWORD)" | Out-Null
if($LASTEXITCODE -ne 0){throw 'Separate Projects databases could not be provisioned'}
