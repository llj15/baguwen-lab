param(
    [Parameter(Position = 0)]
    [string]$Lab,

    [string]$ResultsDir,
    [string]$Compose = "docker compose",
    [switch]$KeepContainers,
    [switch]$List,
    [switch]$Help
)

$ErrorActionPreference = "Stop"

function Show-Usage {
    @"
Usage:
  .\scripts\run-lab.ps1 -List
  .\scripts\run-lab.ps1 <lab> [options]

Labs:
  redis-cache-failure       Cache penetration, breakdown, avalanche
  redis-distributed-lock    SET NX EX, Redlock, watchdog renewal

Aliases:
  cache                     redis-cache-failure
  lock                      redis-distributed-lock
  distributed-lock          redis-distributed-lock

Options:
  -ResultsDir DIR           Output directory. Relative paths resolve from repo root.
  -Compose CMD              Compose command, default: docker compose
  -KeepContainers           Keep Compose containers after the run.
  -Help                     Show this help.
"@
}

function Resolve-LabName {
    param([string]$Name)

    switch ($Name) {
        "cache" { "redis-cache-failure"; return }
        "redis-cache-failure" { "redis-cache-failure"; return }
        "lock" { "redis-distributed-lock"; return }
        "distributed-lock" { "redis-distributed-lock"; return }
        "redis-distributed-lock" { "redis-distributed-lock"; return }
        default { throw "Unknown lab: $Name" }
    }
}

if ($Help) {
    Show-Usage
    exit 0
}

if ($List) {
    "redis-cache-failure"
    "redis-distributed-lock"
    exit 0
}

if ([string]::IsNullOrWhiteSpace($Lab)) {
    Show-Usage
    exit 2
}

$repoRoot = Resolve-Path -LiteralPath (Join-Path $PSScriptRoot "..")
$canonicalLab = Resolve-LabName $Lab

if ([string]::IsNullOrWhiteSpace($ResultsDir)) {
    $resolvedResultsDir = Join-Path $repoRoot "tmp-results\$canonicalLab"
}
elseif ([System.IO.Path]::IsPathRooted($ResultsDir)) {
    $resolvedResultsDir = $ResultsDir
}
else {
    $resolvedResultsDir = Join-Path $repoRoot $ResultsDir
}

New-Item -ItemType Directory -Force -Path $resolvedResultsDir | Out-Null

Write-Host "Running lab: $canonicalLab"
Write-Host "Results directory: $resolvedResultsDir"

$labDir = Join-Path $repoRoot "labs\$canonicalLab"
$runScript = Join-Path $labDir "run.ps1"

$runArgs = @{
    ResultsDir = $resolvedResultsDir
    Compose = $Compose
}
if ($KeepContainers) {
    $runArgs.KeepContainers = $true
}

& $runScript @runArgs
