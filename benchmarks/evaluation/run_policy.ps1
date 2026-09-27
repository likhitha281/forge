param(
    [ValidateSet("static", "naive", "cost-aware")]
    [string]$Policy,

    [int]$StateMB = 64,

    [int]$Work = 500000,

    [int]$Trials = 1,

    [string]$Forge = ".\forge.exe",

    [int]$InitialGPU = 2,

    [int]$TargetGPU = 4,

    [int]$NaiveDelaySeconds = 5,

    [int]$JobTimeoutSeconds = 1800
)

$ErrorActionPreference = "Stop"

$repoRoot =
    Resolve-Path (
        Join-Path $PSScriptRoot "..\.."
    )

$resultsDir =
    Join-Path $repoRoot "results"

New-Item `
    -ItemType Directory `
    -Force `
    -Path $resultsDir |
    Out-Null

$resultsFile =
    Join-Path `
        $resultsDir `
        "policy_evaluation.csv"

function Invoke-Forge {
    param(
        [string[]]$Arguments
    )

    $output =
        & $Forge @Arguments 2>&1

    if ($LASTEXITCODE -ne 0) {
        throw (
            "Forge command failed: {0}`n{1}" -f `
                ($Arguments -join " "),
                ($output | Out-String)
        )
    }

    return (
        $output |
            Out-String
    )
}

function Get-JobId {
    param(
        [string]$Output
    )

    $match =
        [regex]::Match(
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

        [int]$TimeoutSeconds = 120
    )

    $deadline =
        (Get-Date).AddSeconds(
            $TimeoutSeconds
        )

    while (
        (Get-Date) -lt $deadline
    ) {
        $status =
            Invoke-Forge @(
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
            throw (
                "Job $JobId became terminal while waiting for GPU $GPU`n$status"
            )
        }

        Start-Sleep `
            -Milliseconds 500
    }

    throw (
        "Timed out waiting for job $JobId at GPU $GPU"
    )
}

function Request-Transition {
    param(
        [string]$JobId,

        [int]$TargetGPU
    )

    $output =
        Invoke-Forge @(
            "transition",
            $JobId,
            "--gpus",
            "$TargetGPU"
        )

    $match =
        [regex]::Match(
            $output,
            '(?m)^ID:\s+([0-9a-fA-F-]{36})\s*$'
        )

    if (-not $match.Success) {
        throw (
            "Could not parse transition ID:`n$output"
        )
    }

    return $match.Groups[1].Value
}

function Wait-ForTransition {
    param(
        [string]$TransitionId,

        [int]$TimeoutSeconds = 120
    )

    $deadline =
        (Get-Date).AddSeconds(
            $TimeoutSeconds
        )

    while (
        (Get-Date) -lt $deadline
    ) {
        $sql = @"
SELECT state
FROM transitions
WHERE id = '$TransitionId';
"@

        $state =
            docker compose exec -T postgres `
                psql -U forge -d forge `
                -t -A `
                -c $sql

        if ($LASTEXITCODE -ne 0) {
            throw (
                "Failed to query transition $TransitionId"
            )
        }

        $state =
            $state.Trim()

        if ($state -eq "COMPLETED") {
            return
        }

        if ($state -eq "FAILED") {
            throw (
                "Transition $TransitionId failed"
            )
        }

        Start-Sleep `
            -Milliseconds 250
    }

    throw (
        "Timed out waiting for transition $TransitionId"
    )
}

function Wait-ForTerminalJob {
    param(
        [string]$JobId,

        [int]$TimeoutSeconds
    )

    $deadline =
        (Get-Date).AddSeconds(
            $TimeoutSeconds
        )

    while (
        (Get-Date) -lt $deadline
    ) {
        $status =
            Invoke-Forge @(
                "status",
                $JobId
            )

        if (
            $status -match "Status:\s+COMPLETED"
        ) {
            return "COMPLETED"
        }

        if (
            $status -match "Status:\s+FAILED"
        ) {
            throw (
                "Job $JobId failed`n$status"
            )
        }

        if (
            $status -match "Status:\s+CANCELLED"
        ) {
            throw (
                "Job $JobId was unexpectedly cancelled`n$status"
            )
        }

        Start-Sleep `
            -Milliseconds 500
    }

    throw (
        "Timed out waiting for job $JobId to terminate"
    )
}

function Get-TransitionCount {
    param(
        [string]$JobId
    )

    $sql = @"
SELECT COUNT(*)
FROM transitions
WHERE job_id = '$JobId'
  AND state = 'COMPLETED';
"@

    $count =
        docker compose exec -T postgres `
            psql -U forge -d forge `
            -t -A `
            -c $sql

    if ($LASTEXITCODE -ne 0) {
        throw (
            "Failed to count transitions for job $JobId"
        )
    }

    return [int]$count.Trim()
}

function Get-TransitionTotals {
    param(
        [string]$JobId
    )

    $sql = @"
SELECT
    COALESCE(SUM(
        prepare_us +
        checkpoint_us +
        reconfigure_us +
        restore_us +
        resume_us
    ), 0),
    COALESCE(SUM(bytes_moved), 0)
FROM transitions
WHERE job_id = '$JobId'
  AND state = 'COMPLETED';
"@

    $row =
        docker compose exec -T postgres `
            psql -U forge -d forge `
            -t -A `
            -F "|" `
            -c $sql

    if ($LASTEXITCODE -ne 0) {
        throw (
            "Failed to query transition totals for job $JobId"
        )
    }

    $parts =
        $row.Trim().Split("|")

    if ($parts.Count -ne 2) {
        throw (
            "Unexpected transition totals row: $row"
        )
    }

    return [PSCustomObject]@{
        transition_us =
            [int64]$parts[0]

        bytes_moved =
            [int64]$parts[1]
    }
}

function Get-FinalGPU {
    param(
        [string]$JobId
    )

    $sql = @"
SELECT COALESCE(
    (
        SELECT target_gpu_count
        FROM transitions
        WHERE job_id = '$JobId'
          AND state = 'COMPLETED'
        ORDER BY completed_at DESC
        LIMIT 1
    ),
    $InitialGPU
);
"@

    $value =
        docker compose exec -T postgres `
            psql -U forge -d forge `
            -t -A `
            -c $sql

    if ($LASTEXITCODE -ne 0) {
        throw (
            "Failed to determine final GPU for job $JobId"
        )
    }

    return [int]$value.Trim()
}

function Write-Result {
    param(
        [PSCustomObject]$Result
    )

    if (
        Test-Path $resultsFile
    ) {
        $Result |
            Export-Csv `
                -Path $resultsFile `
                -NoTypeInformation `
                -Append
    }
    else {
        $Result |
            Export-Csv `
                -Path $resultsFile `
                -NoTypeInformation
    }
}

for (
    $trial = 1;
    $trial -le $Trials;
    $trial++
) {
    $jobId =
        $null

    $manualTransitionId =
        $null

    Write-Host ""
    Write-Host "========================================="
    Write-Host (
        "policy={0} state={1}MiB work={2} trial={3}" -f `
            $Policy,
            $StateMB,
            $Work,
            $trial
    )
    Write-Host "========================================="

    try {
        $command =
            "elastic --state-mb $StateMB --work $Work"

        $submit =
            Invoke-Forge @(
                "submit",
                $command,
                "--cpu", "2",
                "--memory-mb", "1024",
                "--gpu-min", "$InitialGPU",
                "--gpu-preferred", "$InitialGPU",
                "--gpu-max", "$TargetGPU"
            )

        $jobId =
            Get-JobId $submit

        Write-Host "job=$jobId"

        $submittedAt =
            [DateTime]::UtcNow

        Wait-ForAllocation `
            -JobId $jobId `
            -GPU $InitialGPU `
            -TimeoutSeconds 120

        $startedAt =
            [DateTime]::UtcNow

        Write-Host (
            "job running at GPU {0}" -f `
                $InitialGPU
        )

        if (
            $Policy -eq "naive"
        ) {
            Start-Sleep `
                -Seconds $NaiveDelaySeconds

            $manualTransitionId =
                Request-Transition `
                    -JobId $jobId `
                    -TargetGPU $TargetGPU

            Write-Host (
                "naive transition={0}" -f `
                    $manualTransitionId
            )

            Wait-ForTransition `
                -TransitionId $manualTransitionId `
                -TimeoutSeconds 120

            Wait-ForAllocation `
                -JobId $jobId `
                -GPU $TargetGPU `
                -TimeoutSeconds 120

            Write-Host (
                "naive transition completed"
            )
        }

        if (
            $Policy -eq "cost-aware"
        ) {
            Write-Host (
                "cost-aware: waiting for Forge autoscaler"
            )
        }

        $terminalState =
            Wait-ForTerminalJob `
                -JobId $jobId `
                -TimeoutSeconds $JobTimeoutSeconds

        $completedAt =
            [DateTime]::UtcNow

        $completionSeconds =
            (
                $completedAt -
                $startedAt
            ).TotalSeconds

        $queueSeconds =
            (
                $startedAt -
                $submittedAt
            ).TotalSeconds

        $transitionCount =
            Get-TransitionCount `
                -JobId $jobId

        $transitionTotals =
            Get-TransitionTotals `
                -JobId $jobId

        $finalGPU =
            Get-FinalGPU `
                -JobId $jobId

        $result =
            [PSCustomObject]@{
                timestamp_utc =
                    [DateTime]::UtcNow.ToString(
                        "o"
                    )

                policy =
                    $Policy

                trial =
                    $trial

                state_mb =
                    $StateMB

                work =
                    $Work

                job_id =
                    $jobId

                initial_gpu =
                    $InitialGPU

                target_gpu =
                    $TargetGPU

                final_gpu =
                    $finalGPU

                transition_count =
                    $transitionCount

                manual_transition_id =
                    if (
                        $null -eq
                        $manualTransitionId
                    ) {
                        ""
                    }
                    else {
                        $manualTransitionId
                    }

                queue_seconds =
                    [math]::Round(
                        $queueSeconds,
                        3
                    )

                completion_seconds =
                    [math]::Round(
                        $completionSeconds,
                        3
                    )

                transition_seconds =
                    [math]::Round(
                        (
                            $transitionTotals.transition_us /
                            1000000.0
                        ),
                        6
                    )

                bytes_moved =
                    $transitionTotals.bytes_moved

                final_state =
                    $terminalState
            }

        Write-Result `
            -Result $result

        Write-Host (
            "completed policy={0} time={1}s transitions={2} final_gpu={3}" -f `
                $Policy,
                $result.completion_seconds,
                $result.transition_count,
                $result.final_gpu
        )
    }
    catch {
        Write-Warning (
            "FAILED policy={0} state={1}MiB work={2} trial={3}: {4}" -f `
                $Policy,
                $StateMB,
                $Work,
                $trial,
                $_.Exception.Message
        )

        continue
    }
}

Write-Host ""
Write-Host "Evaluation complete."
Write-Host "Results: $resultsFile"