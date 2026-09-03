[CmdletBinding()]
param(
    [switch]$User,
    [switch]$WithZot,
    [string]$UserLocusRoot
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

. (Join-Path $PSScriptRoot 'internal/locus-paths.ps1')

if (-not $User -and -not [string]::IsNullOrWhiteSpace($UserLocusRoot)) {
    throw '-UserLocusRoot requires -User'
}

$RepositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
if ($User) {
    $LocusRoot = Resolve-LocusUserRoot -Override $UserLocusRoot
    if ($WithZot) {
        & (Join-Path $PSScriptRoot 'zot.ps1') uninstall -User -UserLocusRoot $LocusRoot
    }

    $BinaryRoot = Join-Path $LocusRoot 'bin'
    foreach ($FileName in 'locus-scope', 'locus-scope.exe', 'locus-pkg', 'locus-pkg.exe') {
        $Target = Join-Path $BinaryRoot $FileName
        if (Test-Path -LiteralPath $Target -PathType Leaf) {
            Remove-Item -LiteralPath $Target -Force
            Write-Output "Removed $Target"
        }
    }
    if ((Test-Path -LiteralPath $BinaryRoot -PathType Container) -and
        $null -eq (Get-ChildItem -LiteralPath $BinaryRoot -Force | Select-Object -First 1)) {
        Remove-Item -LiteralPath $BinaryRoot -Force
    }
    if ((Test-Path -LiteralPath $LocusRoot -PathType Container) -and
        $null -eq (Get-ChildItem -LiteralPath $LocusRoot -Force | Select-Object -First 1)) {
        Remove-Item -LiteralPath $LocusRoot -Force
    }
    return
}

$Targets = @(
    (Join-Path $RepositoryRoot 'temp/build'),
    (Join-Path $RepositoryRoot 'temp/local')
)
foreach ($Target in $Targets) {
    if (Test-Path -LiteralPath $Target) {
        Remove-Item -LiteralPath $Target -Recurse -Force
        Write-Output "Removed $Target"
    }
}
if ($WithZot) {
    & (Join-Path $PSScriptRoot 'zot.ps1') uninstall
}
