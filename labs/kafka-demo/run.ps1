param(
    [string]$ResultsDir = "./results",
    [string]$Compose = "docker compose",
    [switch]$KeepContainers
)

$ErrorActionPreference = "Stop"
Set-Location -LiteralPath $PSScriptRoot

$composeArgs = $Compose -split " "
New-Item -ItemType Directory -Force -Path $ResultsDir | Out-Null

function Invoke-Checked {
    param(
        [string]$Command,
        [string[]]$Arguments
    )

    & $Command @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "Command failed with exit code ${LASTEXITCODE}: $Command $($Arguments -join ' ')"
    }
}

function Invoke-Compose {
    param([string[]]$Arguments)

    $command = $composeArgs[0]
    $prefix = @()
    if ($composeArgs.Length -gt 1) {
        $prefix = $composeArgs[1..($composeArgs.Length - 1)]
    }
    Invoke-Checked $command ($prefix + $Arguments)
}

try {
    Write-Host "=========================================="
    Write-Host "  Kafka demo lab run"
    Write-Host "=========================================="
    Write-Host "Results directory: $ResultsDir"

    Write-Host "`n[1/4] Cleaning previous compose containers..."
    try {
        Invoke-Compose @("down", "--remove-orphans")
    }
    catch {
        Write-Warning $_
    }

    Write-Host "`n[2/4] Building and running experiment..."
    $env:RESULTS_DIR = $ResultsDir
    Invoke-Compose @("up", "--build", "--abort-on-container-exit", "--exit-code-from", "experiment", "experiment")

    Write-Host "`n[3/4] Generating analysis artifacts..."
    Invoke-Compose @("up", "--build", "--no-deps", "--abort-on-container-exit", "--exit-code-from", "analysis", "analysis")

    Write-Host "`n[4/4] Verifying theory invariants..."
    Invoke-Compose @("run", "--rm", "--no-deps", "analysis", "python", "/app/scripts/verify_results.py", "/data/results.json")
}
finally {
    if (-not $KeepContainers) {
        try {
            Invoke-Compose @("down", "--remove-orphans") | Out-Null
        }
        catch {
            Write-Warning $_
        }
    }
}
