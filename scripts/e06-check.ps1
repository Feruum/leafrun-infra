param([string]$DockerContext='desktop-linux',[switch]$Race)
. (Join-Path $PSScriptRoot 'e06-common.ps1') -DockerContext $DockerContext
foreach($service in @('postgres','kratos','access-api-a','access-api-b')){Get-E06Container $service | Out-Null}
& (Join-Path $PSScriptRoot 'contexts.ps1')
Invoke-E06Compose --profile checks config --quiet
Invoke-E06Compose --profile checks build access-checks
Invoke-E06Compose --profile checks up --detach --wait kratos-expiry
try {
 $testArguments=@('-count=1','-v','./...','../contracts/access/v1','../platform/authn')
 if($Race){$testArguments=@('-race')+$testArguments}
 Invoke-E06Compose --profile checks run --rm --no-deps access-checks @testArguments
} finally {Get-E06Container kratos-expiry | Out-Null;Invoke-E06Compose --profile checks stop kratos-expiry}
