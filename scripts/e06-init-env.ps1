param([string]$DockerContext='desktop-linux')
. (Join-Path $PSScriptRoot 'e06-common.ps1') -DockerContext $DockerContext
function New-E06Secret([int]$Bytes=32){$value=[byte[]]::new($Bytes);[Security.Cryptography.RandomNumberGenerator]::Fill($value);return [Convert]::ToHexString($value).ToLowerInvariant()}
$e06Values=@{}
if(Test-Path -LiteralPath $e06EnvFile){$e06Values=Read-E06Secrets}
foreach($key in @('E06_POSTGRES_ADMIN_PASSWORD','E06_ACCESS_DB_PASSWORD','E06_KRATOS_DB_PASSWORD','E06_KETO_DB_PASSWORD','E06_KRATOS_COOKIE_SECRET','E06_PROJECTS_SERVICE_TOKEN','E06_GENERATION_SERVICE_TOKEN','E06_GENERATION_DB_PASSWORD','E06_PROJECTS_DB_PASSWORD','E06_STORAGE_DB_PASSWORD')){if(-not $e06Values.ContainsKey($key)){$e06Values[$key]=New-E06Secret};if($e06Values[$key] -notmatch '^[0-9a-f]{64}$'){throw 'Existing E06 secret has invalid format; refusing replacement'}}
if(-not $e06Values.ContainsKey('E06_KRATOS_CIPHER_SECRET')){$e06Values.E06_KRATOS_CIPHER_SECRET=New-E06Secret 16}
foreach($key in @('E06_TEMPORAL_DB_PASSWORD','E06_STORAGE_SERVICE_TOKEN','E06_GENERATION_STORAGE_TOKEN','E06_PROJECTS_STORAGE_TOKEN','E06_OWNER_ROUTER_TOKEN','E06_GENERATION_OWNER_TOKEN','E06_PROJECTS_OWNER_TOKEN','E06_PROJECTS_GENERATION_TOKEN','E06_IMPORT_OPERATOR_TOKEN','E06_IMPORT_PROJECTS_TOKEN','E06_IMPORT_STORAGE_TOKEN','E06_S3_ACCESS_KEY','E06_S3_SECRET_KEY','E06_S3_TEST_ACCESS_KEY','E06_S3_TEST_SECRET_KEY','E06_S3_ADMIN_ACCESS_KEY','E06_S3_ADMIN_SECRET_KEY')){if(-not $e06Values.ContainsKey($key)){$e06Values[$key]=New-E06Secret};if($e06Values[$key] -notmatch '^[0-9a-f]{64}$'){throw 'Existing E06 owner credential invalid; refusing replacement'}}
if($e06Values.E06_KRATOS_CIPHER_SECRET -notmatch '^[0-9a-f]{32}$'){throw 'Existing E06 cipher secret must be exactly 32 ASCII characters'}
foreach($key in @('E06_HYDRA_DB_PASSWORD','E06_HYDRA_SYSTEM_SECRET','E06_HYDRA_COOKIE_SECRET','E06_ACCESS_OAUTH_KEY_HEX')){if(-not $e06Values.ContainsKey($key)){$e06Values[$key]=New-E06Secret};if($e06Values[$key] -notmatch '^[0-9a-f]{64}$'){throw 'Existing E06 OAuth secret invalid; refusing replacement'}}
if(-not $e06Values.ContainsKey('E06_ACCESS_OAUTH_KEY_ID')){$e06Values.E06_ACCESS_OAUTH_KEY_ID='e06-oauth-v1'}
if($e06Values.E06_ACCESS_OAUTH_KEY_ID -notmatch '^[a-zA-Z0-9_-]{1,64}$'){throw 'Existing E06 OAuth key identifier invalid; refusing replacement'}
foreach($key in @('E06_AUDIT_DB_PASSWORD','E06_AUDIT_SERVICE_TOKEN','E06_AUDIT_CURSOR_KEY_HEX','E06_AUDIT_OPERATOR_TOKEN','E06_NATS_ACCESS_PASSWORD','E06_NATS_GENERATION_PASSWORD','E06_NATS_PROJECTS_PASSWORD','E06_NATS_STORAGE_PASSWORD','E06_NATS_AUDIT_PASSWORD','E06_NATS_ADMIN_PASSWORD')){if(-not $e06Values.ContainsKey($key)){$e06Values[$key]=New-E06Secret};if($e06Values[$key] -notmatch '^[0-9a-f]{64}$'){throw 'Existing E06 Audit credential invalid; refusing replacement'}}
if(-not $e06Values.ContainsKey('E06_AUDIT_OPERATOR_AUTHORITY_ID')){$e06Values.E06_AUDIT_OPERATOR_AUTHORITY_ID=[Guid]::NewGuid().ToString()}
if($e06Values.E06_AUDIT_OPERATOR_AUTHORITY_ID -notmatch '^[0-9a-f-]{36}$'){throw 'Existing Audit operator authority identifier invalid'}
$lines=foreach($key in ($e06Values.Keys|Sort-Object)){"$key=$($e06Values[$key])"}
Write-E06PrivateText $e06EnvFile (($lines -join "`n")+"`n")
[IO.Directory]::CreateDirectory((Join-Path $e06InfraRoot '.cache/e06'))|Out-Null
$e06S3Identities=@(
 @{name='e06-bootstrap';credentials=@(@{accessKey=$e06Values.E06_S3_ADMIN_ACCESS_KEY;secretKey=$e06Values.E06_S3_ADMIN_SECRET_KEY});actions=@('Admin','Read','Write','List','Tagging')},
 @{name='e06-storage';credentials=@(@{accessKey=$e06Values.E06_S3_ACCESS_KEY;secretKey=$e06Values.E06_S3_SECRET_KEY});actions=@('Read:leafrun-e06-storage','Write:leafrun-e06-storage','List:leafrun-e06-storage','Tagging:leafrun-e06-storage')},
 @{name='e06-storage-test';credentials=@(@{accessKey=$e06Values.E06_S3_TEST_ACCESS_KEY;secretKey=$e06Values.E06_S3_TEST_SECRET_KEY});actions=@('Read:leafrun-e06-storage-test','Write:leafrun-e06-storage-test','List:leafrun-e06-storage-test','Tagging:leafrun-e06-storage-test')})
$e06S3Path=Join-Path $e06InfraRoot '.cache/e06/s3.json';Write-E06PrivateText $e06S3Path (@{identities=$e06S3Identities}|ConvertTo-Json -Depth 8)
$e06NatsUsers=@(
 @{user='access';password=$e06Values.E06_NATS_ACCESS_PASSWORD;permissions=@{publish=@{allow=@('leafrun.audit.access.action.v1','$JS.API.INFO')};subscribe=@{allow=@('_INBOX.access.>')}}},
 @{user='generation';password=$e06Values.E06_NATS_GENERATION_PASSWORD;permissions=@{publish=@{allow=@('leafrun.generation.job.completed.v1','leafrun.audit.generation.action.v1','$JS.API.INFO')};subscribe=@{allow=@('_INBOX.generation.>')}}},
 @{user='projects';password=$e06Values.E06_NATS_PROJECTS_PASSWORD;permissions=@{publish=@{allow=@('leafrun.projects.release.candidate.v1','leafrun.projects.release.ready.v1','leafrun.projects.release.failed.v1','leafrun.projects.release.published.v1','leafrun.projects.release.withdrawn.v1','leafrun.projects.release.deleted.v1','leafrun.audit.projects.action.v1','$JS.API.INFO')};subscribe=@{allow=@('_INBOX.projects.>')}}},
 @{user='storage';password=$e06Values.E06_NATS_STORAGE_PASSWORD;permissions=@{publish=@{allow=@('leafrun.audit.storage.action.v1','$JS.API.INFO')};subscribe=@{allow=@('_INBOX.storage.>')}}},
 @{user='audit';password=$e06Values.E06_NATS_AUDIT_PASSWORD;permissions=@{publish=@{allow=@('$JS.API.INFO','$JS.API.STREAM.INFO.LEAFRUN_AUDIT','$JS.API.CONSUMER.INFO.LEAFRUN_AUDIT.LEAFRUN_AUDIT_V1','$JS.API.CONSUMER.MSG.NEXT.LEAFRUN_AUDIT.LEAFRUN_AUDIT_V1','$JS.ACK.LEAFRUN_AUDIT.LEAFRUN_AUDIT_V1.>')};subscribe=@{allow=@('_INBOX.audit.>')}}},
 @{user='bootstrap';password=$e06Values.E06_NATS_ADMIN_PASSWORD;permissions=@{publish=@{allow=@('>')};subscribe=@{allow=@('>')}}}
)
$e06NatsPath=Join-Path $e06InfraRoot '.cache/e06/nats.json'
Write-E06PrivateText $e06NatsPath (@{port=4222;http_port=8222;jetstream=@{store_dir='/data'};authorization=@{users=$e06NatsUsers}}|ConvertTo-Json -Depth 10)
Remove-Variable e06NatsUsers,e06NatsPath -ErrorAction SilentlyContinue
Remove-Variable e06Values,lines,key,e06S3Identities -ErrorAction SilentlyContinue
Write-Output 'Private E06 credentials ready; all existing runtime keys preserved'
