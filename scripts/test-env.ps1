# Dot-source this file to configure host-side tests without exposing credentials.
$ErrorActionPreference = 'Stop'
$infraRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$taskRoot = [IO.Path]::GetFullPath((Join-Path $infraRoot '..'))
. (Join-Path $PSScriptRoot 'workspace.ps1')
$cfg=@{}
Get-Content -LiteralPath (Join-Path $infraRoot '.env') | ForEach-Object { $key,$value=$_ -split '=',2; $cfg[$key]=$value }
$env:AWS_ACCESS_KEY_ID=$cfg.S3_ACCESS_KEY
$env:AWS_SECRET_ACCESS_KEY=$cfg.S3_SECRET_KEY
$env:AWS_REGION='us-east-1'
$env:AWS_EC2_METADATA_DISABLED='true'
$env:LEAFRUN_STORAGE_DATABASE_URL="postgres://storage:$($cfg.STORAGE_DB_PASSWORD)@127.0.0.1:58432/storage_test?sslmode=disable"
$env:LEAFRUN_STORAGE_S3_ENDPOINT='http://127.0.0.1:58333'
$env:LEAFRUN_STORAGE_S3_BUCKET='leafrun-storage'
$env:LEAFRUN_STORAGE_OWNER_URL='http://127.0.0.1:58082'
$env:LEAFRUN_STORAGE_OWNER_TOKEN=$cfg.OWNER_TOKEN
$env:LEAFRUN_STORAGE_SERVICE_CREDENTIALS=@(@{token=$cfg.STORAGE_TOKEN;subject='demo';domain='core'}) | ConvertTo-Json -Compress -AsArray
$env:LEAFRUN_STORAGE_TEMPORAL_ADDRESS='127.0.0.1:57233'
$env:LEAFRUN_STORAGE_INTEGRATION='1'
$env:LEAFRUN_STORAGE_TEMPORAL_INTEGRATION='1'
$env:LEAFRUN_E03_INTEGRATION='1'
$env:LEAFRUN_E03_STORAGE_URL='http://127.0.0.1:58081'
$env:LEAFRUN_E03_OWNER_URL='http://127.0.0.1:58082'
$env:LEAFRUN_E03_FONT_FILE=Join-Path $taskRoot 'core/examples/invoice/fonts/FiraSans-Regular.ttf'
$env:STORAGE_TOKEN=$cfg.STORAGE_TOKEN
$env:FIXTURE_ADMIN_TOKEN=$cfg.FIXTURE_ADMIN_TOKEN
$env:LEAFRUN_DEV_FIXTURE='true'
# These are source repositories created by the sandbox, not arbitrary trusted trees.
$repos=@($infraRoot,(Join-Path $taskRoot 'storage'),(Join-Path $taskRoot 'core'),(Join-Path $taskRoot 'contracts'))
$env:GIT_CONFIG_COUNT="$($repos.Count)"
for($i=0;$i -lt $repos.Count;$i++){[Environment]::SetEnvironmentVariable("GIT_CONFIG_KEY_$i",'safe.directory','Process');[Environment]::SetEnvironmentVariable("GIT_CONFIG_VALUE_$i",$repos[$i].Replace('\','/'),'Process')}
