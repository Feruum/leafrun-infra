$ErrorActionPreference = 'Stop'
$target = Join-Path $PSScriptRoot '../.env'
function New-Secret { $bytes = [byte[]]::new(32); [Security.Cryptography.RandomNumberGenerator]::Fill($bytes); [Convert]::ToHexString($bytes).ToLowerInvariant() }
$keys = @('POSTGRES_ADMIN_PASSWORD','STORAGE_DB_PASSWORD','TEMPORAL_DB_PASSWORD','S3_ACCESS_KEY','S3_SECRET_KEY','S3_TEST_ACCESS_KEY','S3_TEST_SECRET_KEY','S3_ADMIN_ACCESS_KEY','S3_ADMIN_SECRET_KEY','STORAGE_TOKEN','OWNER_TOKEN','FIXTURE_ADMIN_TOKEN')
$existing = @{}
if (Test-Path -LiteralPath $target) { Get-Content -LiteralPath $target | ForEach-Object { $key,$value=$_ -split '=',2; $existing[$key]=$value } }
$lines = foreach ($key in $keys) { if (-not $existing[$key]) { $existing[$key]=New-Secret }; "$key=$($existing[$key])" }
[IO.File]::WriteAllText($target, (($lines -join "`n") + "`n"))
$runtime = Join-Path $PSScriptRoot '../.cache'
[IO.Directory]::CreateDirectory($runtime) | Out-Null
$identities = @(
 @{name='bootstrap'; credentials=@(@{accessKey=$existing.S3_ADMIN_ACCESS_KEY; secretKey=$existing.S3_ADMIN_SECRET_KEY}); actions=@('Admin','Read','Write','List','Tagging')},
 @{name='storage'; credentials=@(@{accessKey=$existing.S3_ACCESS_KEY; secretKey=$existing.S3_SECRET_KEY}); actions=@('Read:leafrun-storage','Write:leafrun-storage','List:leafrun-storage','Tagging:leafrun-storage')},
 @{name='storage-test'; credentials=@(@{accessKey=$existing.S3_TEST_ACCESS_KEY; secretKey=$existing.S3_TEST_SECRET_KEY}); actions=@('Read:leafrun-storage-test','Write:leafrun-storage-test','List:leafrun-storage-test','Tagging:leafrun-storage-test')}
)
[IO.File]::WriteAllText((Join-Path $runtime 's3.json'),(@{identities=$identities} | ConvertTo-Json -Depth 8))
Write-Output 'Ignored local credentials and separate S3 roles ready; existing values preserved'
