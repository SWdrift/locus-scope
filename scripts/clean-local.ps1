[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$RepositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$Targets = @(
    (Join-Path $RepositoryRoot 'temp\build'),
    (Join-Path $RepositoryRoot 'temp\local')
)

foreach ($Target in $Targets) {
    if (Test-Path -LiteralPath $Target) {
        Remove-Item -LiteralPath $Target -Recurse -Force
        Write-Output "Removed $Target"
    }
}
