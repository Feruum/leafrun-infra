param([string]$DockerContext='desktop-linux')
. (Join-Path $PSScriptRoot 'e06-common.ps1') -DockerContext $DockerContext
foreach($ownedService in @('postgres','kratos','access-api-a','access-api-b')){Get-E06Container $ownedService|Out-Null}
$fixturePath=Join-Path $e06InfraRoot '.cache/e06/browser-fixture.json'
$fixture=[IO.File]::ReadAllText($fixturePath)|ConvertFrom-Json
$receipt=[ordered]@{project='leafrun-e06';startedAt=[DateTimeOffset]::UtcNow.ToString('o');before=@();accessRestart=@();providerRestart=@();outage=@();restored=@();nativeLogout=@()}
function Request-E06([string]$Method,[string]$Address,[hashtable]$Headers=@{},[string]$Body=''){
 try {return Invoke-WebRequest -Uri $Address -Method $Method -Headers $Headers -Body $Body -ContentType application/json -TimeoutSec 8 -SkipHttpErrorCheck}catch{throw 'Owned E06 HTTP check failed'}
}
function Assert-E06Sessions([string]$Stage,[int]$Expected){
 foreach($base in @('http://127.0.0.1:58197','http://127.0.0.1:58198')){
  $response=Request-E06 GET "$base/v1/session" @{Authorization="Bearer $script:e06SessionToken"}
  if($response.StatusCode -ne $Expected){throw "E06 session status mismatch at $Stage"}
  $entry=@{node=$base;status=[int]$response.StatusCode}
  if($Expected -eq 200){$resolved=$response.Content|ConvertFrom-Json;$identityJSON=$resolved.identity|ConvertTo-Json -Depth 5 -Compress;if($script:e06ExpectedIdentity -and $script:e06ExpectedIdentity -ne $identityJSON){throw 'Authority changed across E06 nodes/restart'};$script:e06ExpectedIdentity=$identityJSON;if($resolved.identity.principal.subject -ne $fixture.identityId){throw 'Unexpected E06 fixture actor'};$entry.identity=$resolved.identity;$entry.expiresAt=$resolved.expiresAt}
  $receipt[$Stage]+=$entry
 }
}
function Wait-E06Ready {
 foreach($base in @('http://127.0.0.1:58197','http://127.0.0.1:58198')){$ready=$false;for($attempt=0;$attempt -lt 30;$attempt++){try{$response=Request-E06 GET "$base/readyz";if($response.StatusCode -eq 200){$ready=$true;break}}catch{};Start-Sleep -Milliseconds 500};if(-not $ready){throw 'E06 readiness did not recover'}}
}
$flow=Request-E06 GET 'http://127.0.0.1:54433/self-service/login/api' @{Accept='application/json'}
if($flow.StatusCode -ne 200){throw 'E06 native flow unavailable'}
$flowData=$flow.Content|ConvertFrom-Json
$flowAction=[Uri]$flowData.ui.action
if($flowData.type -ne 'api' -or $flowAction.GetLeftPart([UriPartial]::Authority) -ne 'http://127.0.0.1:54433' -or $flowAction.AbsolutePath -ne '/self-service/login'){throw 'Unexpected native fixture form action'}
$body=@{method='password';identifier=$fixture.email;password=$fixture.password}|ConvertTo-Json -Compress
$login=Request-E06 POST $flowData.ui.action @{Accept='application/json'} $body
if($login.StatusCode -ne 200){throw 'E06 native fixture login failed'}
$loginData=$login.Content|ConvertFrom-Json
$script:e06SessionToken=$loginData.session_token
if(-not $script:e06SessionToken){throw 'E06 native fixture token missing'}
$script:e06ExpectedIdentity=''
try {
 Assert-E06Sessions before 200
 Invoke-E06Compose restart access-api-a access-api-b | Out-Null
 Wait-E06Ready
 Assert-E06Sessions accessRestart 200
 Invoke-E06Compose restart kratos | Out-Null
 Wait-E06Ready
 Assert-E06Sessions providerRestart 200
 Invoke-E06Compose run --rm --no-deps access-migrate | Out-Null
 Assert-E06Sessions providerRestart 200
 Invoke-E06Compose stop kratos | Out-Null
 try {
  Assert-E06Sessions outage 503
  foreach($base in @('http://127.0.0.1:58197','http://127.0.0.1:58198')){if((Request-E06 GET "$base/healthz").StatusCode -ne 200 -or (Request-E06 GET "$base/readyz").StatusCode -ne 503){throw 'E06 outage liveness/readiness contract failed'}}
 } finally {Invoke-E06Compose start kratos | Out-Null;Wait-E06Ready}
 Assert-E06Sessions restored 200
} finally {
 $logoutBody=@{session_token=$script:e06SessionToken}|ConvertTo-Json -Compress
 $logout=Request-E06 DELETE 'http://127.0.0.1:54433/self-service/logout/api' @{Accept='application/json'} $logoutBody
 if($logout.StatusCode -ne 204){throw 'E06 native fixture logout failed'}
 Assert-E06Sessions nativeLogout 401
 Remove-Variable fixture,body,login,loginData,flowData,logoutBody -ErrorAction SilentlyContinue
 Remove-Variable e06SessionToken,e06ExpectedIdentity -Scope Script -ErrorAction SilentlyContinue
}
$receipt.completedAt=[DateTimeOffset]::UtcNow.ToString('o')
$receiptPath=Join-Path $e06TaskRoot 'reviews/task02/12-restart-outage-receipt.json'
[IO.File]::WriteAllText($receiptPath,($receipt|ConvertTo-Json -Depth 8)+"`n")
Write-Output 'E06 shared authority survived both API/provider restarts and migration rerun; both nodes failed closed503 during provider outage and recovered; native logout denied both401'
