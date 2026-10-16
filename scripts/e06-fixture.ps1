param([string]$DockerContext='desktop-linux')
. (Join-Path $PSScriptRoot 'e06-common.ps1') -DockerContext $DockerContext
$e06FixturePath=Join-Path $e06InfraRoot '.cache/e06/browser-fixture.json'
$e06Kratos=Get-E06Container kratos
if(Test-Path -LiteralPath $e06FixturePath){Set-E06PrivateFile $e06FixturePath;Write-Output 'Existing private E06 browser fixture preserved';return}
$bytes=[byte[]]::new(24);[Security.Cryptography.RandomNumberGenerator]::Fill($bytes)
$password=[Convert]::ToHexString($bytes)+'aA!'
$email='browser-'+[Guid]::NewGuid().ToString('N')+'@e06.example.test'
$body=@{schema_id='default';state='active';external_id='leafrun-e06-browser-'+[Guid]::NewGuid().ToString('N');traits=@{email=$email};credentials=@{password=@{config=@{password=$password}}}}|ConvertTo-Json -Depth 6 -Compress
$response=$body|& docker --context $DockerContext exec -i $e06Kratos wget -q -O - --header=Content-Type:application/json --post-file=/dev/stdin http://127.0.0.1:4434/admin/identities 2>$null
if($LASTEXITCODE -ne 0){throw 'E06 private fixture creation failed'}
try{$identity=$response|ConvertFrom-Json}catch{throw 'E06 private fixture response invalid'}
if($identity.id -notmatch '^[0-9a-f-]{36}$' -or $identity.state -ne 'active'){throw 'E06 private fixture identity invalid'}
[IO.File]::WriteAllText($e06FixturePath,(@{identityId=$identity.id;email=$email;password=$password;accessA='http://127.0.0.1:58197';accessB='http://127.0.0.1:58198';kratosPublic='http://127.0.0.1:54433'}|ConvertTo-Json -Compress)+"`n")
Set-E06PrivateFile $e06FixturePath
Remove-Variable password,email,body,response,identity,bytes -ErrorAction SilentlyContinue
Write-Output 'Private E06 browser fixture created; values were not printed'
