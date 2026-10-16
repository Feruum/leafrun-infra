param([string]$DockerContext='desktop-linux',[string[]]$GoArguments=@('-json','./e06'),[ValidateSet('','seed','cleanup')][string]$BrowserAction='', [ValidateSet('task05','task07')][string]$BrowserFixture='task05', [switch]$OAuthBrowserOwner, [ValidatePattern('^leafrun-e06-[a-z0-9-]+$')][string]$TestContainerName='leafrun-e06-owner-checks',[string]$AuditOutageDirectory='', [string]$TypstBinary=$env:LEAFRUN_LINUX_TYPST_BINARY, [switch]$AllModules,[switch]$Sandbox)
. (Join-Path $PSScriptRoot 'e06-common.ps1') -DockerContext $DockerContext
& (Join-Path $PSScriptRoot 'e06-owner-test-env.ps1') -DockerContext $DockerContext
$e06OwnerEnvFile=Join-Path $e06InfraRoot '.cache/e06/owner-tests.env'
$e06Mounts=@()
if($AuditOutageDirectory){
 $e06AuditDirectory=[IO.Path]::GetFullPath($AuditOutageDirectory)
 $e06AuditParent=[IO.Path]::GetFullPath((Join-Path $e06InfraRoot '.cache/e06'))
 if([IO.Path]::GetDirectoryName($e06AuditDirectory) -ne $e06AuditParent -or [IO.Path]::GetFileName($e06AuditDirectory) -notmatch '^audit-outage-[0-9a-f]{32}$' -or -not(Test-Path -LiteralPath $e06AuditDirectory -PathType Container)){throw 'Audit outage evidence must be a fresh bounded owned fixture directory'}
 $e06Mounts+=@('--mount',"type=bind,source=$e06AuditDirectory,target=/audit-outage",'-e','E06_AUDIT_OUTAGE_DIRECTORY=/audit-outage')
}
foreach($e06Repo in @('infra','access','audit','contracts','platform','core','generation','projects','storage')){$e06Source=[IO.Path]::GetFullPath((Join-Path $e06TaskRoot $e06Repo));$e06Mounts+=@('--mount',"type=bind,source=$e06Source,target=/work/$e06Repo,readonly")}
$e06Integrations=[IO.Path]::GetFullPath((Join-Path $e06TaskRoot '../integrations'));if(Test-Path -LiteralPath (Join-Path $e06TaskRoot 'integrations/go.mod')){$e06Integrations=Join-Path $e06TaskRoot 'integrations'}
if($env:LEAFRUN_INTEGRATIONS_DIR){$e06Integrations=[IO.Path]::GetFullPath($env:LEAFRUN_INTEGRATIONS_DIR)}
if(-not(Test-Path -LiteralPath (Join-Path $e06Integrations 'go.mod'))){throw 'E06 requires the integrations checkout'}
$e06Mounts+=@('--mount',"type=bind,source=$e06Integrations,target=/work/integrations,readonly")
$e06ToolsRoot=[IO.Path]::GetFullPath((Join-Path $e06TaskRoot '../../../.tools'))
$e06Typst=if($TypstBinary){[IO.Path]::GetFullPath($TypstBinary)}else{Join-Path $e06ToolsRoot 'typst-x86_64-unknown-linux-musl/typst'};if(-not(Test-Path -LiteralPath $e06Typst -PathType Leaf)){throw 'Pinned Linux Typst binary unavailable; supply -TypstBinary'}
$e06Mounts+=@('--mount',"type=bind,source=$e06Typst,target=/tools/typst,readonly")
$e06BrowserDirectory=Join-Path $e06InfraRoot ".cache/e06/$BrowserFixture-browser"
if(-not(Test-Path -LiteralPath $e06BrowserDirectory)){New-Item -ItemType Directory -Path $e06BrowserDirectory|Out-Null}
if($BrowserAction){$e06BrowserActionKey=if($BrowserFixture -eq 'task07'){'E06_OAUTH_BROWSER_ACTION'}else{'E06_BROWSER_ACTION'};$e06Mounts+=@('--mount',"type=bind,source=$e06BrowserDirectory,target=/evidence",'-e',"${e06BrowserActionKey}=$BrowserAction")}
if($OAuthBrowserOwner){if($BrowserFixture -ne 'task07' -or $BrowserAction){throw 'OAuth owner browser check requires its independent Task7 fixture'};$e06Mounts+=@('--mount',"type=bind,source=$e06BrowserDirectory,target=/evidence",'-e','E06_OAUTH_BROWSER_OWNER=true','-p','127.0.0.1:58200:58200')}
$e06CheckScript=if($AllModules){'/work/infra/scripts/e06-verify.sh'}else{'/work/infra/scripts/e06-owner-checks.sh'}
if($AllModules){$e06Mounts+=@('-e','E06_REPLICA_INTEGRATION=1')}
if($Sandbox){
 $e06SandboxImage=& docker --context $DockerContext image inspect leafrun-e06-renderer:source --format '{{.Id}}'
 if($LASTEXITCODE -ne 0 -or $e06SandboxImage -notmatch '^sha256:[0-9a-f]{64}$'){throw 'Immutable E06 renderer image unavailable for sandbox qualification'}
 $e06Mounts+=@('--mount','type=bind,source=/var/run/docker.sock,target=/var/run/docker.sock','-e','LEAFRUN_E04_SANDBOX_INTEGRATION=1','-e',"LEAFRUN_RENDERER_IMAGE=$e06SandboxImage")
}
& docker --context $DockerContext run --rm --name $TestContainerName --network leafrun-e06_default --env-file $e06OwnerEnvFile @e06Mounts --mount type=volume,source=leafrun-e06-go-mod,target=/go/pkg/mod --mount type=volume,source=leafrun-e06-go-build,target=/root/.cache/go-build golang:1.27.0-bookworm@sha256:ded31c68586d2e49e760acc2e65a884b23d032e9bbbed0ae0c55abd3fcaf4452 sh $e06CheckScript @GoArguments
if($LASTEXITCODE -ne 0){throw 'Owned E06 owner qualification failed; preserve this output as failed'}
