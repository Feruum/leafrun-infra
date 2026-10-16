param([string]$DockerContext='desktop-linux')
$ErrorActionPreference='Stop'
& (Join-Path $PSScriptRoot 'e06-init-env.ps1') -DockerContext $DockerContext
. (Join-Path $PSScriptRoot 'e06-common.ps1') -DockerContext $DockerContext
& (Join-Path $PSScriptRoot 'contexts.ps1')
Invoke-E06Compose config --quiet
Invoke-E06Compose up --detach --wait postgres
& (Join-Path $PSScriptRoot 'e06-database.ps1') -DockerContext $DockerContext
Invoke-E06Compose build access-migrate oauth-demo
$e06KratosMigration=Invoke-E06Compose run --rm kratos-migrate 2>&1
$e06KetoMigration=Invoke-E06Compose run --rm keto-migrate 2>&1
$e06HydraMigration=Invoke-E06Compose run --rm hydra-migrate 2>&1
Remove-Variable e06KratosMigration,e06KetoMigration,e06HydraMigration -ErrorAction SilentlyContinue
Invoke-E06Compose run --rm access-migrate | Out-Null
Invoke-E06Compose up --detach --no-build --wait kratos keto hydra
& (Join-Path $PSScriptRoot 'e06-hydra-client.ps1') -DockerContext $DockerContext
Invoke-E06Compose up --detach --no-build access-reconcile access-api-a access-api-b oauth-demo
foreach($address in @('http://127.0.0.1:58197/readyz','http://127.0.0.1:58198/readyz')){
 $ready=$false
 for($attempt=0;$attempt -lt 60;$attempt++){try{$response=Invoke-WebRequest $address -TimeoutSec 2 -SkipHttpErrorCheck;if($response.StatusCode -eq 200){$ready=$true;break}}catch{};Start-Sleep -Seconds 1}
 if(-not $ready){throw 'E06 Access readiness failed'}
}
Write-Output 'Owned E06 stand ready: Access 58197/58198, Kratos 54433, Hydra 54444 on 127.0.0.1; exact demo localhost:58200; provider admins remain private'
