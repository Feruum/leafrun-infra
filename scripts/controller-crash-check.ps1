param([string]$DockerContext = 'desktop-linux')

$ErrorActionPreference = 'Stop'
$infraRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$receiptPath = Join-Path $infraRoot '.cache/e04-controller-crash-receipt.json'
$servicesNeedRestore = $false
Push-Location $infraRoot

function Invoke-Docker {
    param([Parameter(Mandatory)][string[]]$Arguments)
    $result = & docker --context $DockerContext @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "docker --context $DockerContext $($Arguments -join ' ') failed with exit code $LASTEXITCODE"
    }
    return $result
}

function Get-ServiceContainerID {
    param([Parameter(Mandatory)][string]$Service)
    $ids = @(Invoke-Docker -Arguments @('compose', '-p', 'leafrun-e04', 'ps', '--all', '-q', $Service))
    if ($ids.Count -ne 1) {
        throw "Expected one Compose container for $Service; found $($ids.Count)"
    }
    $id = ([string]$ids[0]).Trim()
    if ($id -notmatch '^[0-9a-f]{64}$') {
        throw "Compose returned an invalid container ID for $Service"
    }
    return $id
}

function Get-ContainerInspection {
    param([Parameter(Mandatory)][string]$ID)
    $json = Invoke-Docker -Arguments @('inspect', $ID)
    return (($json -join "`n") | ConvertFrom-Json)[0]
}

function Get-JobContainerIDs {
    param([Parameter(Mandatory)][string]$JobID)
    $ids = Invoke-Docker -Arguments @('ps', '--all', '--no-trunc', '--filter', "label=leafrun.job=$JobID", '--format', '{{.ID}}')
    return @($ids | ForEach-Object { ([string]$_).Trim() } | Where-Object { $_ })
}

function Assert-RendererPolicy {
    param([Parameter(Mandatory)]$Inspection, [Parameter(Mandatory)][string]$JobID)
    $labels = $Inspection.Config.Labels
    if ($labels.'leafrun.job' -ne $JobID -or [string]::IsNullOrWhiteSpace($labels.'leafrun.attempt')) {
        throw 'Renderer container labels do not identify exactly the submitted job and its attempt'
    }
    $binds = @()
    if ($null -ne $Inspection.HostConfig.Binds) {
        $binds = @($Inspection.HostConfig.Binds)
    }
    $mounts = @()
    if ($null -ne $Inspection.Mounts) {
        $mounts = @($Inspection.Mounts)
    }
    $reportedWorkTmpfsMounts = @($mounts | Where-Object { $_.Type -eq 'tmpfs' -and $_.Destination -eq '/work' })
    $unexpectedMounts = @($mounts | Where-Object { $_.Type -ne 'tmpfs' -or $_.Destination -ne '/work' })
    if ($binds.Count -ne 0 -or $reportedWorkTmpfsMounts.Count -gt 1 -or $unexpectedMounts.Count -ne 0) {
        throw 'Renderer container has a bind, volume, or unexpected mount; only the /work tmpfs is allowed'
    }
    $tmpfsDestinations = @()
    if ($null -ne $Inspection.HostConfig.Tmpfs) {
        $tmpfsDestinations = @($Inspection.HostConfig.Tmpfs.PSObject.Properties | ForEach-Object { $_.Name })
    }
    if ($tmpfsDestinations.Count -ne 1 -or $tmpfsDestinations[0] -ne '/work') {
        throw 'Renderer HostConfig.Tmpfs must contain only /work'
    }
    if ($Inspection.Config.Image -notmatch '^sha256:[0-9a-f]{64}$' -or
        $Inspection.Config.User -ne '65532:65532' -or
        ($Inspection.Config.Entrypoint -join ' ') -ne '/leafrun-supervisor' -or
        ($Inspection.Config.Cmd -join ' ') -ne '--timeout=60s' -or
        $Inspection.HostConfig.Memory -ne 536870912 -or
        $Inspection.HostConfig.NanoCpus -ne 1000000000 -or
        $Inspection.HostConfig.NetworkMode -ne 'none' -or
        -not $Inspection.HostConfig.ReadonlyRootfs -or
        $Inspection.HostConfig.Tmpfs.'/work' -notmatch '(^|,)size=268435456(,|$)' -or
        $Inspection.HostConfig.RestartPolicy.Name -ne 'no') {
        throw 'Renderer container differs from the immutable image, PID 1 timeout, or fixed isolation/resource policy'
    }
    if (-not $Inspection.State.Running) {
        throw 'Renderer container stopped before controller termination'
    }
}

function Invoke-TypstCanary {
    $typstCommand = Get-Command typst -ErrorAction Stop
    $tempRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath())
    $dir = Join-Path $tempRoot ('leafrun-e04-canary-' + [guid]::NewGuid().ToString('N'))
    $resolvedDir = [IO.Path]::GetFullPath($dir)
    $separator = [IO.Path]::DirectorySeparatorChar
    $tempRootPrefix = $tempRoot.TrimEnd([IO.Path]::DirectorySeparatorChar, [IO.Path]::AltDirectorySeparatorChar) + $separator
    $comparison = if ([Environment]::OSVersion.Platform -eq [PlatformID]::Win32NT) { [StringComparison]::OrdinalIgnoreCase } else { [StringComparison]::Ordinal }
    if (-not $resolvedDir.StartsWith($tempRootPrefix, $comparison) -or
        [IO.Path]::GetFileName($resolvedDir) -notmatch '^leafrun-e04-canary-[0-9a-f]{32}$') {
        throw 'Refusing to create or recursively remove an unexpected Typst canary directory'
    }
    [IO.Directory]::CreateDirectory($dir) | Out-Null
    $sourcePath = Join-Path $dir 'main.typ'
    $pdfPath = Join-Path $dir 'main.pdf'
    $stdoutPath = Join-Path $dir 'stdout.txt'
    $stderrPath = Join-Path $dir 'stderr.txt'
    $source = @'
#let a = 0
#while a < 10000 {
  a = a + 1
  let b = 0
  while b < 10000 {
    b = b + 1
    let c = 0
    while c < 10000 {
      c = c + 1
      ()
    }
    ()
  }
  ()
}
'@
    [IO.File]::WriteAllText($sourcePath, $source.TrimStart("`n"), [Text.UTF8Encoding]::new($false))
    $process = $null
    try {
        $startArguments = @{
            FilePath = $typstCommand.Source
            ArgumentList = @('compile', $sourcePath, $pdfPath)
            PassThru = $true
            RedirectStandardOutput = $stdoutPath
            RedirectStandardError = $stderrPath
        }
        if ([Environment]::OSVersion.Platform -eq [PlatformID]::Win32NT) {
            $startArguments.WindowStyle = 'Hidden'
        }
        $process = Start-Process @startArguments
        if ($process.WaitForExit(2000)) {
            throw "Typst canary completed before two seconds (exit code $($process.ExitCode)); refusing to submit the watchdog job"
        }
        Stop-Process -Id $process.Id -Force -ErrorAction SilentlyContinue
        $process.WaitForExit()
        if (Get-Process -Id $process.Id -ErrorAction SilentlyContinue) {
            throw 'Typst canary process did not stop after the bounded check'
        }
    } finally {
        if ($process -and -not $process.HasExited) {
            Stop-Process -Id $process.Id -Force -ErrorAction SilentlyContinue
            $process.WaitForExit()
        }
        $resolvedDir = [IO.Path]::GetFullPath($dir)
        $tempRootPrefix = $tempRoot.TrimEnd([IO.Path]::DirectorySeparatorChar, [IO.Path]::AltDirectorySeparatorChar) + [IO.Path]::DirectorySeparatorChar
        if (-not $resolvedDir.StartsWith($tempRootPrefix, $comparison) -or
            [IO.Path]::GetFileName($resolvedDir) -notmatch '^leafrun-e04-canary-[0-9a-f]{32}$') {
            throw 'Refusing to recursively remove an unexpected Typst canary directory'
        }
        Remove-Item -LiteralPath $resolvedDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}

try {
    . ./scripts/test-env.ps1
    if (-not $cfg.GENERATION_DB_PASSWORD) {
        throw 'Internal generation database credentials are unavailable'
    }
    $env:LEAFRUN_E04_CRASH_RECEIPT_FILE = $receiptPath
    $env:LEAFRUN_E04_CRASH_GENERATION_DATABASE_URL = "postgres://generation:$($cfg.GENERATION_DB_PASSWORD)@127.0.0.1:58434/generation?sslmode=disable"

    $osType = Invoke-Docker -Arguments @('info', '--format', '{{.OSType}}')
    if (($osType -join '').Trim() -ne 'linux') {
        throw "Docker context $DockerContext is not using a Linux engine"
    }
    Invoke-TypstCanary
    Write-Output 'Typst CPU-bound canary stayed active past two seconds and was stopped.'

    foreach ($address in @('http://127.0.0.1:58090/readyz', 'http://127.0.0.1:58091/readyz')) {
        $response = Invoke-WebRequest -Uri $address -TimeoutSec 3 -SkipHttpErrorCheck
        if ($response.StatusCode -ne 200 -and $response.StatusCode -ne 204) {
            throw "E04 service readiness failed: $address"
        }
    }

    $controllerID = Get-ServiceContainerID -Service 'generation-controller'
    $janitorID = Get-ServiceContainerID -Service 'generation-janitor'
    $controllerState = Get-ContainerInspection -ID $controllerID
    $janitorState = Get-ContainerInspection -ID $janitorID
    if (-not $controllerState.State.Running -or -not $janitorState.State.Running) {
        throw 'Generation controller and janitor must both be running before the crash check'
    }
    if ($controllerState.HostConfig.RestartPolicy.Name -ne 'unless-stopped' -or $janitorState.HostConfig.RestartPolicy.Name -ne 'unless-stopped') {
        throw 'The generation controller and janitor must have their expected unless-stopped recovery policy'
    }

    # Keep the janitor out of the watchdog interval. A lost controller lease
    # expires after 30s, while PID 1 owns its independent, fixed 60s timeout.
    $servicesNeedRestore = $true
    Invoke-Docker -Arguments @('compose', '-p', 'leafrun-e04', 'stop', 'generation-janitor') | Out-Null
    $janitorState = Get-ContainerInspection -ID $janitorID
    if ($janitorState.State.Running) {
        throw 'Generation janitor did not stop before job submission'
    }

    $env:LEAFRUN_E04_CRASH_STAGE = 'submit'
    & go test -count=1 -timeout=90s -v -run '^TestLiveControllerCrashRecovery$' ./e04
    if ($LASTEXITCODE -ne 0) {
        throw "E04 crash job submission failed with exit code $LASTEXITCODE"
    }
    if (-not (Test-Path -LiteralPath $receiptPath)) {
        throw 'Crash test did not write its public job receipt'
    }
    $receipt = Get-Content -LiteralPath $receiptPath -Raw | ConvertFrom-Json
    $jobID = [string]$receipt.jobId
    if ($jobID -notmatch '^gen_[0-9a-f]{32}$') {
        throw 'Crash receipt has an invalid public job ID'
    }

    $rendererID = $null
    $rendererInspection = $null
    $renderDeadline = [DateTimeOffset]::UtcNow.AddSeconds(25)
    $top = @()
    while ([DateTimeOffset]::UtcNow -lt $renderDeadline) {
        $ids = @(Get-JobContainerIDs -JobID $jobID)
        if ($ids.Count -gt 1) {
            throw 'More than one managed renderer container appeared for the crash job'
        }
        if ($ids.Count -eq 1) {
            $rendererID = $ids[0]
            if ($rendererID -notmatch '^[0-9a-f]{64}$') {
                throw 'Docker returned an invalid renderer container ID'
            }
            $rendererInspection = Get-ContainerInspection -ID $rendererID
            if ($rendererInspection.State.Running) {
                $top = Invoke-Docker -Arguments @('top', $rendererID, '-eo', 'pid,comm')
                if (($top -join "`n") -match '(?im)^\s*\d+\s+typst\s*$') {
                    Assert-RendererPolicy -Inspection $rendererInspection -JobID $jobID
                    break
                }
            }
        }
        Start-Sleep -Seconds 1
    }
    if (-not $rendererID -or -not $rendererInspection.State.Running -or (($top -join "`n") -notmatch '(?im)^\s*\d+\s+typst\s*$')) {
        throw 'The submitted Typst renderer did not reach a running state within 25 seconds'
    }

    $janitorState = Get-ContainerInspection -ID $janitorID
    if ($janitorState.State.Running) {
        throw 'The generation janitor restarted before controller termination'
    }
    Write-Output 'One correctly isolated renderer is running Typst; killing only the generation controller with SIGKILL.'
    $controllerKilledAt = [DateTimeOffset]::UtcNow
    Invoke-Docker -Arguments @('kill', '--signal', 'SIGKILL', $controllerID) | Out-Null
    $controllerStopDeadline = [DateTimeOffset]::UtcNow.AddSeconds(8)
    do {
        $controllerAfterKill = Get-ContainerInspection -ID $controllerID
        if (-not $controllerAfterKill.State.Running -and $controllerAfterKill.State.Status -eq 'exited') {
            break
        }
        Start-Sleep -Milliseconds 250
    } while ([DateTimeOffset]::UtcNow -lt $controllerStopDeadline)
    if ($controllerAfterKill.State.Running -or $controllerAfterKill.State.Status -ne 'exited') {
        throw 'Generation controller did not remain stopped after its single SIGKILL'
    }

    $watchdogDeadline = [DateTimeOffset]::UtcNow.AddSeconds(65)
    $watchdogState = $null
    while ([DateTimeOffset]::UtcNow -lt $watchdogDeadline) {
        $watchdogState = Get-ContainerInspection -ID $rendererID
        if (-not $watchdogState.State.Running -and $watchdogState.State.Status -eq 'exited') {
            break
        }
        $janitorState = Get-ContainerInspection -ID $janitorID
        if ($janitorState.State.Running) {
            throw 'The generation janitor restarted during the watchdog interval'
        }
        Start-Sleep -Seconds 1
    }
    if (-not $watchdogState -or $watchdogState.State.Running -or $watchdogState.State.Status -ne 'exited') {
        throw 'The renderer PID 1 did not stop the child within 70 seconds after controller death'
    }
    $watchdogElapsed = [DateTimeOffset]::UtcNow - $controllerKilledAt
    $containerLifetime = [DateTimeOffset]::UtcNow - [DateTimeOffset]::Parse($rendererInspection.State.StartedAt)
    if ($watchdogState.State.OOMKilled -or $watchdogState.State.ExitCode -ne 124 -or $watchdogElapsed.TotalSeconds -lt 45 -or $containerLifetime.TotalSeconds -lt 55) {
        throw "Renderer stopped without independent PID 1 timeout proof (status=$($watchdogState.State.Status), exit=$($watchdogState.State.ExitCode), oom=$($watchdogState.State.OOMKilled), elapsed=$([int]$watchdogElapsed.TotalSeconds)s)"
    }
    Write-Output "Renderer exited with PID 1 watchdog code 124 after $([int]$watchdogElapsed.TotalSeconds)s; container lifetime was $([int]$containerLifetime.TotalSeconds)s and janitor remained stopped."

    # Only now may janitor recover the expired database lease and persist the
    # terminal resource_limit classification. The controller remains down.
    Invoke-Docker -Arguments @('start', $janitorID) | Out-Null
    $env:LEAFRUN_E04_CRASH_STAGE = 'verify'
    & go test -count=1 -timeout=90s -v -run '^TestLiveControllerCrashRecovery$' ./e04
    if ($LASTEXITCODE -ne 0) {
        throw "E04 crash recovery verification failed with exit code $LASTEXITCODE"
    }
    $rendererCleanupDeadline = [DateTimeOffset]::UtcNow.AddSeconds(15)
    $remainingRendererIDs = @(Get-JobContainerIDs -JobID $jobID)
    while ($remainingRendererIDs.Count -gt 0 -and [DateTimeOffset]::UtcNow -lt $rendererCleanupDeadline) {
        Start-Sleep -Seconds 1
        $remainingRendererIDs = @(Get-JobContainerIDs -JobID $jobID)
    }
    if ($remainingRendererIDs.Count -gt 0) {
        throw "Renderer cleanup left $($remainingRendererIDs.Count) container(s) for the recovered job"
    }
    Write-Output 'Janitor recovery produced one stopped attempt, resource_limit failure, and no PDF artifact.'
} finally {
    try {
        if ($servicesNeedRestore) {
            $janitorCurrent = Get-ContainerInspection -ID $janitorID
            if (-not $janitorCurrent.State.Running) {
                Invoke-Docker -Arguments @('start', $janitorID) | Out-Null
            }
            $controllerCurrent = Get-ContainerInspection -ID $controllerID
            if (-not $controllerCurrent.State.Running) {
                Invoke-Docker -Arguments @('start', $controllerID) | Out-Null
            }
            Write-Output 'Generation controller and janitor restored.'
        }
    } finally {
        Remove-Item Env:LEAFRUN_E04_CRASH_STAGE, Env:LEAFRUN_E04_CRASH_RECEIPT_FILE, Env:LEAFRUN_E04_CRASH_GENERATION_DATABASE_URL -ErrorAction SilentlyContinue
        Pop-Location
    }
}
