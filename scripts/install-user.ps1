[CmdletBinding()]
param(
    [string]$SetupPath,
    [ValidateSet('scope', 'pkg', 'zot')]
    [string[]]$Components = @('scope', 'pkg', 'zot'),
    [switch]$NoPath,
    [switch]$ZotAutoStart,
    [switch]$Interactive
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$RepositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
if ([string]::IsNullOrWhiteSpace($SetupPath)) {
    $SetupPath = Join-Path $RepositoryRoot 'temp\release\windows-amd64\locus-setup-windows-amd64.exe'
}
$SetupPath = (Resolve-Path -LiteralPath $SetupPath).Path
if ([IO.Path]::GetExtension($SetupPath) -ne '.exe') {
    throw "setup path must name an .exe file: $SetupPath"
}

$selectedComponents = @($Components | Select-Object -Unique)
if ($selectedComponents.Count -eq 0) {
    throw 'at least one component must be selected'
}
if ($ZotAutoStart -and 'zot' -notin $selectedComponents) {
    throw '-ZotAutoStart requires the zot component'
}

$selectedTasks = @()
if (-not $NoPath -and ($selectedComponents -contains 'scope' -or $selectedComponents -contains 'pkg')) {
    $selectedTasks += 'addpath'
}
if ($ZotAutoStart) {
    $selectedTasks += 'zotautostart'
}

$arguments = @(
    '/NORESTART',
    "/COMPONENTS=$($selectedComponents -join ',')",
    "/TASKS=$($selectedTasks -join ',')",
    "/LOG=$(Join-Path $RepositoryRoot 'temp\install-user.log')"
)
if (-not $Interactive) {
    $arguments += '/VERYSILENT', '/SUPPRESSMSGBOXES'
}

$process = Start-Process -FilePath $SetupPath -ArgumentList $arguments -Wait -PassThru
if ($process.ExitCode -ne 0) {
    throw "Locus installer exited with code $($process.ExitCode); see temp/install-user.log"
}

Write-Output "Installed Locus components '$($selectedComponents -join ',')' for the current user"
