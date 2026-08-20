[CmdletBinding()]
param(
    [ValidateSet('demo', 'platform')]
    [string]$Mode = $(if ([string]::IsNullOrWhiteSpace($env:LOCAL_PRINT_AGENT_PRINTER_MODE)) { 'demo' } else { $env:LOCAL_PRINT_AGENT_PRINTER_MODE.Trim().ToLowerInvariant() }),

    [string]$BrowserPath = $env:LOCAL_PRINT_AGENT_BROWSER_PATH,

    [string]$SumatraPath = $env:LOCAL_PRINT_AGENT_SUMATRA_PATH,

    [string]$GoCachePath = $env:GOCACHE
)

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot

function Resolve-OptionalExecutable {
    param(
        [string]$Value,
        [string]$Label
    )

    if ([string]::IsNullOrWhiteSpace($Value)) {
        return ''
    }
    $resolved = Resolve-Path -LiteralPath $Value.Trim() -ErrorAction SilentlyContinue
    if ($null -eq $resolved -or -not (Test-Path -LiteralPath $resolved.Path -PathType Leaf)) {
        throw "$Label does not exist or is not a file: $Value"
    }
    return $resolved.Path
}

if (-not (Test-Path -LiteralPath (Join-Path $repoRoot 'go.mod') -PathType Leaf)) {
    throw "Repository root is invalid: $repoRoot"
}
if ($null -eq (Get-Command go -CommandType Application -ErrorAction SilentlyContinue)) {
    throw 'Go was not found on PATH. Install Go 1.23 or newer.'
}

$resolvedBrowser = Resolve-OptionalExecutable -Value $BrowserPath -Label 'Chrome/Chromium executable'
$resolvedSumatra = ''
if ($Mode -eq 'platform') {
    $resolvedSumatra = Resolve-OptionalExecutable -Value $SumatraPath -Label 'SumatraPDF executable'
    if ([string]::IsNullOrWhiteSpace($resolvedSumatra)) {
        throw 'Platform mode on Windows requires -SumatraPath or LOCAL_PRINT_AGENT_SUMATRA_PATH.'
    }
}
$resolvedGoCache = ''
if (-not [string]::IsNullOrWhiteSpace($GoCachePath)) {
    $cacheCandidate = [System.IO.Path]::GetFullPath($GoCachePath.Trim())
    New-Item -ItemType Directory -Force -Path $cacheCandidate | Out-Null
    $resolvedGoCache = (Resolve-Path -LiteralPath $cacheCandidate).Path
}

$oldMode = $env:LOCAL_PRINT_AGENT_PRINTER_MODE
$oldBrowser = $env:LOCAL_PRINT_AGENT_BROWSER_PATH
$oldSumatra = $env:LOCAL_PRINT_AGENT_SUMATRA_PATH
$oldGoCache = $env:GOCACHE
try {
    $env:LOCAL_PRINT_AGENT_PRINTER_MODE = $Mode
    if ($resolvedBrowser) {
        $env:LOCAL_PRINT_AGENT_BROWSER_PATH = $resolvedBrowser
    } else {
        Remove-Item Env:LOCAL_PRINT_AGENT_BROWSER_PATH -ErrorAction SilentlyContinue
    }
    if ($resolvedSumatra) {
        $env:LOCAL_PRINT_AGENT_SUMATRA_PATH = $resolvedSumatra
    } else {
        Remove-Item Env:LOCAL_PRINT_AGENT_SUMATRA_PATH -ErrorAction SilentlyContinue
    }
    if ($resolvedGoCache) {
        $env:GOCACHE = $resolvedGoCache
    }

    Set-Location -LiteralPath $repoRoot
    if ($Mode -eq 'demo') {
        Write-Host 'Starting in demo mode: PDF previews are real; Mock Printer never submits to an OS queue.'
    } else {
        Write-Host 'Starting in platform mode: jobs may be submitted to the selected Windows printer queue.'
    }
    if (-not $resolvedBrowser) {
        Write-Host 'No browser path was supplied; the agent will use PATH and common install locations.'
    }
    Write-Host 'Open the loopback URL printed below in your browser.'
    & go run .\cmd\local-print-agent
    if ($LASTEXITCODE -ne 0) {
        throw "go run exited with code $LASTEXITCODE"
    }
} finally {
    $env:LOCAL_PRINT_AGENT_PRINTER_MODE = $oldMode
    $env:LOCAL_PRINT_AGENT_BROWSER_PATH = $oldBrowser
    $env:LOCAL_PRINT_AGENT_SUMATRA_PATH = $oldSumatra
    $env:GOCACHE = $oldGoCache
}
