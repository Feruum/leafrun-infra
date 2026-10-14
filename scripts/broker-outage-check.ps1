param([string]$DockerContext = 'desktop-linux')

$ErrorActionPreference = 'Stop'
$infraRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$receiptPath = Join-Path $infraRoot '.cache/e04-broker-outage-receipt.json'
$natsContainerID = $null
$natsWasRunning = $false
$relayContainerID = $null
$consumerContainerID = $null
Push-Location $infraRoot

function Invoke-Docker {
    param([Parameter(Mandatory)][string[]]$Arguments)
    $result = & docker --context $DockerContext @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "docker --context $DockerContext $($Arguments -join ' ') failed with exit code $LASTEXITCODE"
    }
    return $result
}

function Get-ContainerInspection {
    param([Parameter(Mandatory)][string]$ID)
    $json = Invoke-Docker -Arguments @('inspect', $ID)
    return (($json -join "`n") | ConvertFrom-Json)[0]
}

function Get-RunningComposeContainerID {
    param([Parameter(Mandatory)][string]$Service)
    $ids = @(Invoke-Docker -Arguments @('compose', '-p', 'leafrun-e04', 'ps', '-q', $Service) |
        ForEach-Object { ([string]$_).Trim() } | Where-Object { $_ })
    if ($ids.Count -ne 1) {
        throw "Expected one running leafrun-e04 container for $Service; found $($ids.Count)"
    }
    $id = $ids[0]
    if ($id -notmatch '^[0-9a-f]{64}$') {
        throw "Compose returned an invalid container ID for $Service"
    }
    return $id
}

function Assert-ComposeContainer {
    param(
        [Parameter(Mandatory)][string]$ID,
        [Parameter(Mandatory)][string]$Service,
        [Parameter(Mandatory)]$Inspection
    )
    if ($Inspection.Config.Labels.'com.docker.compose.project' -ne 'leafrun-e04' -or
        $Inspection.Config.Labels.'com.docker.compose.service' -ne $Service) {
        throw "Container identity does not match leafrun-e04 service $Service"
    }
}

function Wait-NatsHealthy {
    param([Parameter(Mandatory)][string]$ID)
    $deadline = [DateTimeOffset]::UtcNow.AddSeconds(60)
    do {
        $inspection = Get-ContainerInspection -ID $ID
        Assert-ComposeContainer -ID $ID -Service 'nats' -Inspection $inspection
        if ($inspection.State.Running -and $inspection.State.Health.Status -eq 'healthy') {
            return
        }
        Start-Sleep -Milliseconds 500
    } while ([DateTimeOffset]::UtcNow -lt $deadline)
    throw 'The original NATS container did not return to healthy status within 60 seconds'
}

function Wait-RelayAndConsumerRunning {
    $expectedServices = @(
        @{ Service = 'generation-event-relay'; ID = $relayContainerID },
        @{ Service = 'dev-consumer'; ID = $consumerContainerID }
    )
    $deadline = [DateTimeOffset]::UtcNow.AddSeconds(60)
    do {
        $allRunning = $true
        foreach ($expected in $expectedServices) {
            $inspection = Get-ContainerInspection -ID $expected.ID
            Assert-ComposeContainer -ID $expected.ID -Service $expected.Service -Inspection $inspection
            if (-not $inspection.State.Running) {
                $allRunning = $false
                break
            }
        }
        if ($allRunning) {
            foreach ($expected in $expectedServices) {
                $runningID = Get-RunningComposeContainerID -Service $expected.Service
                if ($runningID -ne $expected.ID) {
                    throw "The $($expected.Service) container identity changed during NATS recovery"
                }
            }
            return
        }
        Start-Sleep -Milliseconds 500
    } while ([DateTimeOffset]::UtcNow -lt $deadline)
    throw 'The original Generation relay and independent consumer containers did not return to running status within 60 seconds'
}

function Invoke-BrokerOutageGoTest {
    param([Parameter(Mandatory)][ValidateSet('submit', 'verify')][string]$Stage)
    $env:LEAFRUN_E04_BROKER_STAGE = $Stage
    & go test -count=1 -timeout=120s -v -run '^TestLiveBrokerOutageRecovery$' ./e04
    if ($LASTEXITCODE -ne 0) {
        throw "E04 broker-outage $Stage phase failed with exit code $LASTEXITCODE"
    }
}

try {
    . ./scripts/test-env.ps1
    if (-not $cfg.GENERATION_DB_PASSWORD) {
        throw 'Internal Generation database credentials are unavailable'
    }
    $env:LEAFRUN_E04_BROKER_RECEIPT_FILE = $receiptPath
    $env:LEAFRUN_E04_BROKER_GENERATION_DATABASE_URL = "postgres://generation:$($cfg.GENERATION_DB_PASSWORD)@127.0.0.1:58434/generation?sslmode=disable"

    $osType = Invoke-Docker -Arguments @('info', '--format', '{{.OSType}}')
    if (($osType -join '').Trim() -ne 'linux') {
        throw "Docker context $DockerContext is not using a Linux engine"
    }

    $readyAddresses = @(($env:LEAFRUN_E04_GENERATION_URL + '/readyz'), ($env:LEAFRUN_E04_STORAGE_URL + '/readyz'))
    foreach ($address in $readyAddresses) {
        $response = Invoke-WebRequest -Uri $address -TimeoutSec 3 -SkipHttpErrorCheck
        if ($response.StatusCode -ne 200 -and $response.StatusCode -ne 204) {
            throw "E04 service readiness failed: $address"
        }
    }

    $natsContainerID = Get-RunningComposeContainerID -Service 'nats'
    $natsInspection = Get-ContainerInspection -ID $natsContainerID
    Assert-ComposeContainer -ID $natsContainerID -Service 'nats' -Inspection $natsInspection
    if (-not $natsInspection.State.Running -or $natsInspection.State.Health.Status -ne 'healthy') {
        throw 'The original leafrun-e04 NATS container must be running and healthy before the outage check'
    }
    $natsWasRunning = $true

    $relayContainerID = Get-RunningComposeContainerID -Service 'generation-event-relay'
    $consumerContainerID = Get-RunningComposeContainerID -Service 'dev-consumer'
    foreach ($pair in @(
        @{ Service = 'generation-event-relay'; ID = $relayContainerID },
        @{ Service = 'dev-consumer'; ID = $consumerContainerID }
    )) {
        $inspection = Get-ContainerInspection -ID $pair.ID
        Assert-ComposeContainer -ID $pair.ID -Service $pair.Service -Inspection $inspection
        if (-not $inspection.State.Running) {
            throw "The $($pair.Service) container must be running before the broker outage check"
        }
    }

    $services = @(Invoke-Docker -Arguments @('compose', '-p', 'leafrun-e04', 'ps', '-q', 'nats'))
    if (@($services | Where-Object { ([string]$_).Trim() -eq $natsContainerID }).Count -ne 1) {
        throw 'The captured NATS container is no longer the running leafrun-e04 NATS service'
    }

    Write-Output 'Stopping only the captured leafrun-e04 NATS container.'
    Invoke-Docker -Arguments @('stop', $natsContainerID) | Out-Null
    $stoppedNats = Get-ContainerInspection -ID $natsContainerID
    Assert-ComposeContainer -ID $natsContainerID -Service 'nats' -Inspection $stoppedNats
    if ($stoppedNats.State.Running -or $stoppedNats.State.Status -ne 'exited') {
        throw 'The captured NATS container did not remain stopped'
    }
    Invoke-BrokerOutageGoTest -Stage 'submit'
    if (-not (Test-Path -LiteralPath $receiptPath)) {
        throw 'The submit phase did not write its public artifact receipt'
    }
    Write-Output 'Restarting the same NATS container and waiting for JetStream health.'
    Invoke-Docker -Arguments @('start', $natsContainerID) | Out-Null
    Wait-NatsHealthy -ID $natsContainerID
    Wait-RelayAndConsumerRunning

    Invoke-BrokerOutageGoTest -Stage 'verify'
    Write-Output 'The same successful artifact was retained, its single render attempt remained finished, and the terminal event reached the independent consumer.'
} finally {
    try {
        if ($natsContainerID -and $natsWasRunning) {
            $currentNats = Get-ContainerInspection -ID $natsContainerID
            Assert-ComposeContainer -ID $natsContainerID -Service 'nats' -Inspection $currentNats
            if (-not $currentNats.State.Running) {
                Invoke-Docker -Arguments @('start', $natsContainerID) | Out-Null
            }
            Wait-NatsHealthy -ID $natsContainerID
            Write-Output 'The original NATS container is restored and healthy.'
        }
    } finally {
        Remove-Item Env:LEAFRUN_E04_BROKER_STAGE, Env:LEAFRUN_E04_BROKER_RECEIPT_FILE, Env:LEAFRUN_E04_BROKER_GENERATION_DATABASE_URL -ErrorAction SilentlyContinue
        Pop-Location
    }
}
