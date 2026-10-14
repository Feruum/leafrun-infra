param([Parameter(Mandatory=$true)][string]$TypstBinary,[string]$DockerContext='desktop-linux',[ValidateSet('core','contracts','storage','generation','platform','infra')][string[]]$Repositories=@('core','contracts','storage','generation','platform','infra'))
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'test-env.ps1')
$runtime=Join-Path $infraRoot '.cache'
[IO.Directory]::CreateDirectory($runtime) | Out-Null
$envFile=Join-Path $runtime 'linux-test.env'
$values=@{
 AWS_ACCESS_KEY_ID=$cfg.S3_TEST_ACCESS_KEY; AWS_SECRET_ACCESS_KEY=$cfg.S3_TEST_SECRET_KEY; AWS_REGION='us-east-1'; AWS_EC2_METADATA_DISABLED='true'
 LEAFRUN_E03_STAND_S3_ACCESS_KEY=$cfg.S3_ACCESS_KEY; LEAFRUN_E03_STAND_S3_SECRET_KEY=$cfg.S3_SECRET_KEY
 LEAFRUN_STORAGE_DATABASE_URL="postgres://storage:$($cfg.STORAGE_DB_PASSWORD)@postgres:5432/storage_test?sslmode=disable"
 LEAFRUN_STORAGE_S3_ENDPOINT='http://seaweed:8333'; LEAFRUN_STORAGE_S3_BUCKET='leafrun-storage-test'
 LEAFRUN_STORAGE_OWNER_URL='http://e03-owner-fixture:8082'; LEAFRUN_STORAGE_OWNER_TOKEN=$cfg.OWNER_TOKEN
 LEAFRUN_STORAGE_SERVICE_CREDENTIALS=$env:LEAFRUN_STORAGE_SERVICE_CREDENTIALS
 LEAFRUN_STORAGE_TEMPORAL_ADDRESS='temporal:7233'; LEAFRUN_STORAGE_INTEGRATION='1'; LEAFRUN_STORAGE_TEMPORAL_INTEGRATION='1'
 LEAFRUN_STORAGE_TEMPORAL_NAMESPACE='e03-unit'
 LEAFRUN_E03_INTEGRATION='1'; LEAFRUN_E03_STORAGE_URL='http://e03-storage-api:8081'; LEAFRUN_E03_OWNER_URL='http://e03-owner-fixture:8082'
 GENERATION_TEST_DATABASE_URL="postgres://generation:$($cfg.GENERATION_DB_PASSWORD)@postgres:5432/generation_test?sslmode=disable"
 PLATFORM_TEST_DATABASE_URL="postgres://dev_consumer:$($cfg.CONSUMER_DB_PASSWORD)@postgres:5432/platform_test?sslmode=disable"
 LEAFRUN_E04_EVENTS_INTEGRATION='1'; LEAFRUN_NATS_ADDRESS='nats://nats:4222'; LEAFRUN_NATS_USER='bootstrap'; LEAFRUN_NATS_PASSWORD=$cfg.NATS_ADMIN_PASSWORD
 LEAFRUN_E04_INTEGRATION='1'; LEAFRUN_E04_STORAGE_URL='http://storage-api:8081'; LEAFRUN_E04_GENERATION_URL='http://generation-api:8080'; LEAFRUN_E04_OWNER_URL='http://owner-fixture:8082'; GENERATION_USER_TOKEN=$cfg.GENERATION_USER_TOKEN
 LEAFRUN_E04_CONSUMER_DATABASE_URL="postgres://dev_consumer:$($cfg.CONSUMER_DB_PASSWORD)@postgres:5432/dev_consumer?sslmode=disable"
 LEAFRUN_NATS_GENERATION_PASSWORD=$cfg.NATS_GENERATION_PASSWORD; LEAFRUN_NATS_CONSUMER_PASSWORD=$cfg.NATS_CONSUMER_PASSWORD
 LEAFRUN_E03_FONT_FILE='/work/core/examples/invoice/fonts/FiraSans-Regular.ttf'
 STORAGE_TOKEN=$cfg.STORAGE_TOKEN; FIXTURE_ADMIN_TOKEN=$cfg.FIXTURE_ADMIN_TOKEN
 GOMODCACHE='/go/pkg/mod'; GOCACHE='/root/.cache/go-build'; CGO_ENABLED='1'
 LEAFRUN_E03_CHECK_REPOS=($Repositories -join ' ')
}
$lines=foreach($key in $values.Keys){"$key=$($values[$key])"}
[IO.File]::WriteAllText($envFile,($lines -join "`n")+"`n")
$binds=@()
foreach($repo in @('core','contracts','storage','generation','platform','infra')){$binds+=@('--mount',"type=bind,source=$(Join-Path $taskRoot $repo),target=/work/$repo,readonly")}
$integrationRoot=$paths.integrations
$binds+=@('--mount',"type=bind,source=$integrationRoot,target=/work/integrations,readonly",'--mount',"type=bind,source=$([IO.Path]::GetFullPath($TypstBinary)),target=/tools/typst,readonly")
docker --context $DockerContext run --rm --name leafrun-e04-linux-checks --network leafrun-e04_default --env-file $envFile @binds --mount type=volume,source=leafrun-e03_go-mod,target=/go/pkg/mod --mount type=volume,source=leafrun-e03_go-build,target=/root/.cache/go-build golang:1.27.0-bookworm@sha256:ded31c68586d2e49e760acc2e65a884b23d032e9bbbed0ae0c55abd3fcaf4452 sh /work/infra/scripts/linux-checks.sh
if($LASTEXITCODE -ne 0){throw 'Linux race/vet/build checks failed'}
