param([string]$DockerContext='desktop-linux')
. (Join-Path $PSScriptRoot 'e06-common.ps1') -DockerContext $DockerContext
$e06Values=Read-E06Secrets
$e06OwnerTestValues=@{
 E06_OWNER_INTEGRATION='true';E06_INTEGRATION='true';E06_KRATOS_INTERNAL_URL='http://kratos:4433';E06_KRATOS_ADMIN_URL='http://kratos:4434'
 E06_ACCESS_A_INTERNAL_URL='http://access-api-a:8080';E06_ACCESS_B_INTERNAL_URL='http://access-api-b:8080';E06_KETO_READ_URL='http://keto:4466';E06_KETO_WRITE_URL='http://keto:4467'
 E06_KRATOS_EXPIRY_URL='http://kratos-expiry:4433';E06_FONT_FILE='/work/core/examples/invoice/fonts/FiraSans-Regular.ttf'
 E06_HYDRA_ADMIN_URL='http://hydra:4445';E06_HYDRA_INTERNAL_URL='http://hydra:4444';E06_HYDRA_PUBLIC_URL='http://127.0.0.1:54444/'
 E06_OAUTH_CLIENT_ID='leafrun-e06-demo';E06_OAUTH_CALLBACK_URL='http://localhost:58200/oauth/callback';E06_OAUTH_AUDIENCE='leafrun-api'
 E06_OAUTH_SCOPES='leafrun.generate leafrun.status leafrun.source.read leafrun.resource.read leafrun.download offline_access'
 E06_ACCESS_OAUTH_KEY_HEX=$e06Values.E06_ACCESS_OAUTH_KEY_HEX;E06_ACCESS_OAUTH_KEY_ID=$e06Values.E06_ACCESS_OAUTH_KEY_ID
 E06_IMPORT_OPERATOR_TOKEN=$e06Values.E06_IMPORT_OPERATOR_TOKEN;E06_PROJECTS_SERVICE_TOKEN=$e06Values.E06_PROJECTS_SERVICE_TOKEN;E06_GENERATION_SERVICE_TOKEN=$e06Values.E06_GENERATION_SERVICE_TOKEN
 E06_GENERATION_STORAGE_TOKEN=$e06Values.E06_GENERATION_STORAGE_TOKEN;E06_PROJECTS_STORAGE_TOKEN=$e06Values.E06_PROJECTS_STORAGE_TOKEN
 E06_AUDIT_INTEGRATION='1';E06_AUDIT_URL='http://127.0.0.1:58199';E06_AUDIT_NATS_URL='nats://nats:4222'
 E06_AUDIT_OPERATOR_TOKEN=$e06Values.E06_AUDIT_OPERATOR_TOKEN;E06_AUDIT_OPERATOR_ID='e06-audit-operator'
 E06_AUDIT_NATS_STORAGE_PASSWORD=$e06Values.E06_NATS_STORAGE_PASSWORD;E06_AUDIT_NATS_ADMIN_PASSWORD=$e06Values.E06_NATS_ADMIN_PASSWORD
 E06_AUDIT_NATS_ACCESS_PASSWORD=$e06Values.E06_NATS_ACCESS_PASSWORD;E06_AUDIT_NATS_PROJECTS_PASSWORD=$e06Values.E06_NATS_PROJECTS_PASSWORD;E06_AUDIT_NATS_GENERATION_PASSWORD=$e06Values.E06_NATS_GENERATION_PASSWORD
 LEAFRUN_AUDIT_TEST_DATABASE_URL="postgres://audit:$($e06Values.E06_AUDIT_DB_PASSWORD)@postgres:5432/audit_test?sslmode=disable"
}
foreach($e06Domain in @('access','generation','projects','storage','audit')){
 $e06Password=$e06Values["E06_$($e06Domain.ToUpperInvariant())_DB_PASSWORD"]
 $e06OwnerTestValues["E06_$($e06Domain.ToUpperInvariant())_DATABASE_URL"]="postgres://${e06Domain}:$e06Password@postgres:5432/${e06Domain}?sslmode=disable"
 $e06OwnerTestValues["$($e06Domain.ToUpperInvariant())_TEST_DATABASE_URL"]="postgres://${e06Domain}:$e06Password@postgres:5432/${e06Domain}_test?sslmode=disable"
}
$e06OwnerTestFile=Join-Path $e06InfraRoot '.cache/e06/owner-tests.env'
$e06OwnerTestLines=foreach($e06Key in ($e06OwnerTestValues.Keys|Sort-Object)){"$e06Key=$($e06OwnerTestValues[$e06Key])"}
Write-E06PrivateText -Path $e06OwnerTestFile -Text (($e06OwnerTestLines -join "`n")+"`n")
Remove-Variable e06Values,e06OwnerTestValues,e06OwnerTestLines,e06Password -ErrorAction SilentlyContinue
Write-Output 'Private E06 owner qualification environment prepared without credential output'
