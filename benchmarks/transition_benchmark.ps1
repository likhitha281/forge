param(
    [int[]]$StateSizes = @(16, 64, 128, 256),
    [int]$Trials = 10,
    [string]$Forge = ".\forge.exe",
    [string]$Duration = "30s"
)

$ErrorActionPreference = "Stop"

$resultsDir = Join-Path $PSScriptRoot "..\results"

New-Item `
    -ItemType Directory `
    -Force `
    -Path $resultsDir |
    Out-Null

$timestamp = Get-Date -Format "yyyyMMdd-HHmmss"

$resultsFile = Join-Path `
    $resultsDir `
    "transition_costs_$timestamp.csv"

$results = @()

function Get-TransitionMeasurement {
    param(
        [string]$TransitionId,
        [string]$JobId,
        [int]$StateMB,
        [int]$SourceGPU,
        [int]$TargetGPU,
        [int]$Trial
    )

    $sql = @"
SELECT
    state,
    prepare_us,
    checkpoint_us,
    reconfigure_us,
    restore_us,
    resume_us,
    checkpoint_bytes,
    restore_bytes,
    bytes_moved
FROM transitions
WHERE id = '$TransitionId';
"@

    $row = docker compose exec -T postgres `
        psql -U forge -d forge `
        -t -A `
        -F "|" `
        -c $sql

    if ($LASTEXITCODE -ne 0) {
        throw "Failed to query transition $TransitionId"
    }

    $parts = $row.Trim().Split("|")

    if ($parts.Count -ne 9) {
        throw "Unexpected transition row: $row"
    }

    $prepareUS = [int64]$parts[1]
    $checkpointUS = [int64]$parts[2]
    $reconfigureUS = [int64]$parts[3]
    $restoreUS = [int64]$parts[4]
    $resumeUS = [int64]$parts[5]

    $totalUS =
        $prepareUS +
        $checkpointUS +
        $reconfigureUS +
        $restoreUS +
        $resumeUS

    return [PSCustomObject]@{
        timestamp_utc =
            [DateTime]::UtcNow.ToString("o")

        job_id =
            $JobId

        transition_id =
            $TransitionId

        trial =
            $Trial

        state_mb =
            $StateMB

        source_gpu =
            $SourceGPU

        target_gpu =
            $TargetGPU

        gpu_delta =
            $TargetGPU - $SourceGPU

        direction =
            if ($TargetGPU -gt $SourceGPU) {
                "grow"
            }
            else {
                "shrink"
            }

        state =
            $parts[0]

        prepare_us =
            $prepareUS

        checkpoint_us =
            $checkpointUS

        reconfigure_us =
            $reconfigureUS

        restore_us =
            $restoreUS

        resume_us =
            $resumeUS

        total_us =
            $totalUS

        checkpoint_bytes =
            [int64]$parts[6]

        restore_bytes =
            [int64]$parts[7]

        bytes_moved =
            [int64]$parts[8]
    }
}

function Invoke-Forge {
    param([string[]]$Arguments)

    $output = & $Forge @Arguments 2>&1

    if ($LASTEXITCODE -ne 0) {
        throw "Forge command failed: $($Arguments -join ' ')`n$output"
    }

    return ($output | Out-String)
}

function Get-JobId {
    param([string]$Output)

    $match = [regex]::Match(
        $Output,
        '(?m)^ID:\s+([0-9a-fA-F-]{36})\s*$'
    )

    if (-not $match.Success) {
        throw "Could not parse job ID:`n$Output"
    }

    return $match.Groups[1].Value
}

function Wait-ForAllocation {
    param(
        [string]$JobId,
        [int]$GPU,
        [int]$TimeoutSeconds = 60
    )

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)

    while ((Get-Date) -lt $deadline) {
        $status = Invoke-Forge @(
            "status",
            $JobId
        )

        if (
            $status -match "Status:\s+RUNNING" -and
            $status -match "Allocation:.*GPU $GPU"
        ) {
            return
        }

        if (
            $status -match "Status:\s+FAILED" -or
            $status -match "Status:\s+CANCELLED" -or
            $status -match "Status:\s+COMPLETED"
        ) {
            throw "Job $JobId became terminal while waiting for GPU $GPU`n$status"
        }

        Start-Sleep -Milliseconds 500
    }

    throw "Timed out waiting for job $JobId at GPU $GPU"
}

function Request-Transition {
    param(
        [string]$JobId,
        [int]$TargetGPU
    )

    $output = Invoke-Forge @(
        "transition",
        $JobId,
        "--gpus",
        "$TargetGPU"
    )

    $match = [regex]::Match(
        $output,
        '(?m)^ID:\s+([0-9a-fA-F-]{36})\s*$'
    )

    if (-not $match.Success) {
        throw "Could not parse transition ID:`n$output"
    }

    return $match.Groups[1].Value
}

function Wait-ForTransition {
    param(
        [string]$TransitionId,
        [int]$TimeoutSeconds = 60
    )

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)

    while ((Get-Date) -lt $deadline) {
        $sql = @"
SELECT state
FROM transitions
WHERE id = '$TransitionId';
"@

        $state = docker compose exec -T postgres `
            psql -U forge -d forge -t -A -c $sql

        $state = $state.Trim()

        if ($state -eq "COMPLETED") {
            return
        }

        if ($state -eq "FAILED") {
            throw "Transition $TransitionId failed"
        }

        Start-Sleep -Milliseconds 250
    }

    throw "Timed out waiting for transition $TransitionId"
}

function Wait-ForTerminalJob {
    param(
        [string]$JobId,
        [int]$TimeoutSeconds = 30
    )

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)

    while ((Get-Date) -lt $deadline) {
        $status = Invoke-Forge @(
            "status",
            $JobId
        )

        if ($status -match "Status:\s+COMPLETED") {
            return "COMPLETED"
        }

        if ($status -match "Status:\s+FAILED") {
            throw "Job $JobId failed`n$status"
        }

        if ($status -match "Status:\s+CANCELLED") {
            throw "Job $JobId was unexpectedly cancelled`n$status"
        }

        Start-Sleep -Milliseconds 500
    }

    throw "Timed out waiting for job $JobId to terminate"
}

function Set-InitialAllocation {
    param(
        [string]$JobId,
        [int]$TargetGPU
    )

    Wait-ForAllocation `
        -JobId $JobId `
        -GPU 2

    if ($TargetGPU -eq 2) {
        return
    }

    $setupTransition = Request-Transition `
        -JobId $JobId `
        -TargetGPU $TargetGPU

    Write-Host "setup transition=$setupTransition"

    Wait-ForTransition `
        -TransitionId $setupTransition

    Wait-ForAllocation `
        -JobId $JobId `
        -GPU $TargetGPU
}

$transitions = @(
    @{ Source = 1; Target = 2 },
    @{ Source = 2; Target = 4 },
    @{ Source = 4; Target = 2 }
)

foreach ($stateMB in $StateSizes) {
    foreach ($pair in $transitions) {
        for ($trial = 1; $trial -le $Trials; $trial++) {

            $source = $pair.Source
            $target = $pair.Target

            # Reset per-trial identifiers so a failed trial cannot
            # accidentally reuse IDs from the previous trial.
            $jobId = $null
            $transitionId = $null

            Write-Host ""
            Write-Host "========================================="
            Write-Host "state=${stateMB}MiB transition=$source->$target trial=$trial"
            Write-Host "========================================="

            try {
                $command =
                    "checkpointable --state-mb $stateMB --duration $Duration"

                $submit = Invoke-Forge @(
                    "submit",
                    $command,
                    "--cpu", "2",
                    "--memory-mb", "1024",
                    "--gpu-min", "1",
                    "--gpu-preferred", "2",
                    "--gpu-max", "4"
                )

                $jobId = Get-JobId $submit

                Write-Host "job=$jobId"

                Set-InitialAllocation `
                    -JobId $jobId `
                    -TargetGPU $source

                $transitionId = Request-Transition `
                    -JobId $jobId `
                    -TargetGPU $target

                Write-Host "measured transition=$transitionId"

                Wait-ForTransition `
                    -TransitionId $transitionId

                Wait-ForAllocation `
                    -JobId $jobId `
                    -GPU $target

                Write-Host "transition completed"

                $measurement = Get-TransitionMeasurement `
                    -TransitionId $transitionId `
                    -JobId $jobId `
                    -StateMB $stateMB `
                    -SourceGPU $source `
                    -TargetGPU $target `
                    -Trial $trial

                $results += $measurement

                $results |
                    Export-Csv `
                        -Path $resultsFile `
                        -NoTypeInformation

                Write-Host (
                    "measurement total={0}us checkpoint={1}us restore={2}us bytes={3}" -f `
                        $measurement.total_us,
                        $measurement.checkpoint_us,
                        $measurement.restore_us,
                        $measurement.bytes_moved
                )

                $terminalState = Wait-ForTerminalJob `
                    -JobId $jobId

                Write-Host "job completed state=$terminalState"
            }
            catch {
                Write-Warning (
                    "FAILED state={0}MiB transition={1}->{2} trial={3}: {4}" -f `
                        $stateMB,
                        $source,
                        $target,
                        $trial,
                        $_.Exception.Message
                )

                continue
            }
        }
    }
}

Write-Host ""
Write-Host ""
Write-Host "Benchmark complete."
Write-Host "Observations: $($results.Count)"
Write-Host "Results: $resultsFile"