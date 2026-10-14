# Dot-source to load isolated test credentials without writing them to output.
$ErrorActionPreference='Stop'
$e04InfraRoot=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$e04TaskRoot=[IO.Path]::GetFullPath((Join-Path $e04InfraRoot '..'))
$e04Secrets=@{}
Get-Content -LiteralPath (Join-Path $e04InfraRoot '.env') | ForEach-Object { $e04Key,$e04Value=$_ -split '=',2; $e04Secrets[$e04Key]=$e04Value }
$env:PLATFORM_TEST_DATABASE_URL="postgres://dev_consumer:$($e04Secrets.CONSUMER_DB_PASSWORD)@127.0.0.1:58434/platform_test?sslmode=disable"
$env:GENERATION_TEST_DATABASE_URL="postgres://generation:$($e04Secrets.GENERATION_DB_PASSWORD)@127.0.0.1:58434/generation_test?sslmode=disable"
$env:LEAFRUN_NATS_ADDRESS='nats://127.0.0.1:54222'
$env:LEAFRUN_NATS_USER='bootstrap'
$env:LEAFRUN_NATS_PASSWORD=$e04Secrets.NATS_ADMIN_PASSWORD
$env:LEAFRUN_NATS_GENERATION_PASSWORD=$e04Secrets.NATS_GENERATION_PASSWORD
$env:LEAFRUN_NATS_CONSUMER_PASSWORD=$e04Secrets.NATS_CONSUMER_PASSWORD
$env:LEAFRUN_E04_EVENTS_INTEGRATION='1'
$e04WorkspaceRoot=[IO.Path]::GetFullPath((Join-Path $e04TaskRoot '../../..'))
$env:GOCACHE=Join-Path $e04WorkspaceRoot '.cache/go-build'
$env:GOMODCACHE=Join-Path $e04WorkspaceRoot '.cache/go-mod'
$env:GOWORK=Join-Path $e04TaskRoot 'e04.work'
$env:PATH=(Join-Path $e04WorkspaceRoot '.tools/typst-v0.15.1/typst-x86_64-pc-windows-msvc')+[IO.Path]::PathSeparator+$env:PATH
Remove-Variable e04Secrets,e04Key,e04Value -ErrorAction SilentlyContinue
