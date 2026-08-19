$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot
Set-Location -LiteralPath $repoRoot
Write-Host 'Starting local-print-agent. Open the URL printed below in your browser.'
go run .\cmd\local-print-agent
