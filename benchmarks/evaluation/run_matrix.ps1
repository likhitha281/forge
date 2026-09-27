param(
    [int]$Trials = 5,
    [int[]]$WorkSizes = @(100000, 500000, 2000000),
    [int]$StateMB = 64
)

$ErrorActionPreference = "Stop"

$runner =
    Join-Path $PSScriptRoot "run_policy.ps1"

foreach ($work in $WorkSizes) {
    foreach ($policy in @("static", "naive")) {

        Write-Host ""
        Write-Host "#########################################"
        Write-Host "policy=$policy state=${StateMB}MiB work=$work"
        Write-Host "#########################################"

        & $runner `
            -Policy $policy `
            -StateMB $StateMB `
            -Work $work `
            -Trials $Trials
    }
}