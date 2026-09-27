param(
    [int]$StateMB = 64,
    [int]$Work = 3000000,
    [int]$SourceGPU = 2,
    [int]$TargetGPU = 4,
    [int]$SourceSampleSeconds = 15,
    [int]$TargetSampleSeconds = 20,
    [string]$Forge = ".\forge.exe"
)

$ErrorActionPreference = "Stop"

function Invoke-Forge {
    param([string[]]$Arguments)

    $output = & $Forge @Arguments 2>&1

    if ($LASTEXITCODE -ne 0) {
        throw "Forge command failed: $($Arguments -join ' ')`n$output"
    }

    return ($output | Out-String)
}

function Get-Id {
    param([string]$Output)

    $match = [regex]::Match(
        $Output,
        '(?m)^ID:\s+([0-9a-fA-F-]{36})\s*$'
    )

    if (-not $match.Success) {
        throw "Could not parse ID:`n$Output"
    }

    return $match.Groups[1].Value
}

function Wait-ForAllocation {
    param(
        [string]$JobId,
        [int]$GPU,
        [int]$TimeoutSeconds = 120
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
            throw "Seed job became terminal while waiting for GPU $GPU`n$status"
        }

        Start-Sleep -Milliseconds 500
    }

    throw "Timed out waiting for seed job at GPU $GPU"
}

function Wait-ForTransition {
    param(
        [string]$TransitionId,
        [int]$TimeoutSeconds = 120
    )

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)

    while ((Get-Date) -lt $deadline) {
        $sql = @"
SELECT state
FROM transitions
WHERE id = '$TransitionId';
"@

        $state = docker compose exec -T postgres `
            psql -U forge -d forge `
            -t -A `
            -c $sql

        if ($LASTEXITCODE -ne 0) {
            throw "Failed to query transition $TransitionId"
        }

        $state = $state.Trim()

        if ($state -eq "COMPLETED") {
            return
        }

        if ($state -eq "FAILED") {
            throw "Seed transition $TransitionId failed"
        }

        Start-Sleep -Milliseconds 250
    }

    throw "Timed out waiting for seed transition $TransitionId"
}

Write-Host ""
Write-Host "========================================="
Write-Host "Seeding Forge performance profile"
Write-Host "state=${StateMB}MiB GPU $SourceGPU -> $TargetGPU"
Write-Host "========================================="

$command =
    "elastic --state-mb $StateMB --work $Work"

$submit = Invoke-Forge @(
    "submit",
    $command,
    "--cpu", "2",
    "--memory-mb", "1024",
    "--gpu-min", "$SourceGPU",
    "--gpu-preferred", "$SourceGPU",
    "--gpu-max", "$TargetGPU"
)

$jobId = Get-Id $submit

Write-Host "seed job=$jobId"

Wait-ForAllocation `
    -JobId $jobId `
    -GPU $SourceGPU

Write-Host "sampling GPU $SourceGPU for ${SourceSampleSeconds}s"

Start-Sleep -Seconds $SourceSampleSeconds

$sourceProgress =
    docker compose exec -T worker `
        cat "/tmp/forge/$jobId/progress.json"

Write-Host "source telemetry:"
Write-Host $sourceProgress

$transitionOutput = Invoke-Forge @(
    "transition",
    $jobId,
    "--gpus",
    "$TargetGPU"
)

$transitionId =
    Get-Id $transitionOutput

Write-Host "seed transition=$transitionId"

Wait-ForTransition `
    -TransitionId $transitionId

Wait-ForAllocation `
    -JobId $jobId `
    -GPU $TargetGPU

Write-Host "sampling GPU $TargetGPU for ${TargetSampleSeconds}s"

Start-Sleep -Seconds $TargetSampleSeconds

$targetProgress =
    docker compose exec -T worker `
        cat "/tmp/forge/$jobId/progress.json"

Write-Host "target telemetry:"
Write-Host $targetProgress

Write-Host ""
Write-Host "Performance profile seeded."
Write-Host "Do NOT restart the worker before cost-aware evaluation."
Write-Host "Seed job: $jobId"