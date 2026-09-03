[CmdletBinding()]
param(
    [Parameter(Position = 0)]
    [ValidateSet('e2e', 'all')]
    [string]$Suite = 'all'
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$RepositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$ZotScript = Join-Path $PSScriptRoot 'zot.ps1'
$TestTarget = if ($Suite -eq 'e2e') { './test/e2e' } else { './...' }
$RegistryWasRunning = $false
$StartedRegistry = $false
$HadRegistryEnvironment = Test-Path Env:LOCUS_TEST_REGISTRY
$PreviousRegistryEnvironment = if ($HadRegistryEnvironment) { (Get-Item Env:LOCUS_TEST_REGISTRY).Value } else { $null }

try {
    try {
        & $ZotScript status | Out-Null
        $RegistryWasRunning = $true
    }
    catch {
        $RegistryWasRunning = $false
    }

    if (-not $RegistryWasRunning) {
        & $ZotScript start
        $StartedRegistry = $true
    }

    $env:LOCUS_TEST_REGISTRY = 'http://127.0.0.1:18080'
    Push-Location $RepositoryRoot
    try {
        & go test $TestTarget '-count=1'
        if ($LASTEXITCODE -ne 0) {
            throw "go test $TestTarget exited with code $LASTEXITCODE"
        }
    }
    finally {
        Pop-Location
    }
}
finally {
    if ($HadRegistryEnvironment) {
        $env:LOCUS_TEST_REGISTRY = $PreviousRegistryEnvironment
    }
    else {
        Remove-Item Env:LOCUS_TEST_REGISTRY -ErrorAction SilentlyContinue
    }

    if ($StartedRegistry) {
        & $ZotScript stop
    }
}
