$ErrorActionPreference = 'Stop'
$target = Join-Path $PSScriptRoot '../.env'
function New-Secret { $bytes = [byte[]]::new(32); [Security.Cryptography.RandomNumberGenerator]::Fill($bytes); [Convert]::ToHexString($bytes).ToLowerInvariant() }
$keys = @('POSTGRES_ADMIN_PASSWORD','STORAGE_DB_PASSWORD','TEMPORAL_DB_PASSWORD','S3_ACCESS_KEY','S3_SECRET_KEY','S3_TEST_ACCESS_KEY','S3_TEST_SECRET_KEY','S3_ADMIN_ACCESS_KEY','S3_ADMIN_SECRET_KEY','STORAGE_TOKEN','OWNER_TOKEN','FIXTURE_ADMIN_TOKEN','GENERATION_DB_PASSWORD','CONSUMER_DB_PASSWORD','GENERATION_STORAGE_TOKEN','GENERATION_USER_TOKEN','GENERATION_OWNER_TOKEN','NATS_GENERATION_PASSWORD','NATS_CONSUMER_PASSWORD','NATS_ADMIN_PASSWORD')
$keys += @('S3_REGRESSION_ACCESS_KEY','S3_REGRESSION_SECRET_KEY')
$existing = @{}
if (Test-Path -LiteralPath $target) { Get-Content -LiteralPath $target | ForEach-Object { $key,$value=$_ -split '=',2; $existing[$key]=$value } }
$lines = foreach ($key in $keys) { if (-not $existing[$key]) { $existing[$key]=New-Secret }; "$key=$($existing[$key])" }
[IO.File]::WriteAllText($target, (($lines -join "`n") + "`n"))
$runtime = Join-Path $PSScriptRoot '../.cache'
[IO.Directory]::CreateDirectory($runtime) | Out-Null
$identities = @(
 @{name='bootstrap'; credentials=@(@{accessKey=$existing.S3_ADMIN_ACCESS_KEY; secretKey=$existing.S3_ADMIN_SECRET_KEY}); actions=@('Admin','Read','Write','List','Tagging')},
 @{name='storage'; credentials=@(@{accessKey=$existing.S3_ACCESS_KEY; secretKey=$existing.S3_SECRET_KEY}); actions=@('Read:leafrun-storage','Write:leafrun-storage','List:leafrun-storage','Tagging:leafrun-storage')},
 @{name='storage-test'; credentials=@(@{accessKey=$existing.S3_TEST_ACCESS_KEY; secretKey=$existing.S3_TEST_SECRET_KEY}); actions=@('Read:leafrun-storage-test','Write:leafrun-storage-test','List:leafrun-storage-test','Tagging:leafrun-storage-test')},
 @{name='storage-regression'; credentials=@(@{accessKey=$existing.S3_REGRESSION_ACCESS_KEY; secretKey=$existing.S3_REGRESSION_SECRET_KEY}); actions=@('Read:leafrun-storage-regression','Write:leafrun-storage-regression','List:leafrun-storage-regression','Tagging:leafrun-storage-regression')}
)
[IO.File]::WriteAllText((Join-Path $runtime 's3.json'),(@{identities=$identities} | ConvertTo-Json -Depth 8))
$natsConfig=@{
 port=4222;http_port=8222;max_payload=2MB
 jetstream=@{store_dir='/data';max_memory_store=64MB;max_file_store=2GB}
 authorization=@{users=@(
  @{user='generation';password=$existing.NATS_GENERATION_PASSWORD;permissions=@{publish=@{allow=@('leafrun.generation.>','$JS.API.INFO')};subscribe=@{allow=@('_INBOX.>')}}},
  @{user='consumer';password=$existing.NATS_CONSUMER_PASSWORD;permissions=@{publish=@{allow=@('$JS.API.INFO','$JS.API.CONSUMER.INFO.LEAFRUN_GENERATION.DEV_CONSUMER','$JS.API.CONSUMER.MSG.NEXT.LEAFRUN_GENERATION.DEV_CONSUMER','$JS.ACK.LEAFRUN_GENERATION.DEV_CONSUMER.>')};subscribe=@{allow=@('_INBOX.>')}}},
  @{user='bootstrap';password=$existing.NATS_ADMIN_PASSWORD;permissions=@{publish=@{allow=@('>')};subscribe=@{allow=@('>')}}}
 )}
}
[IO.File]::WriteAllText((Join-Path $runtime 'nats.json'),($natsConfig|ConvertTo-Json -Depth 10))
Write-Output 'Ignored local credentials and separate S3 roles ready; existing values preserved'
