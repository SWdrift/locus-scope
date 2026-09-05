[CmdletBinding()]
param(
    [switch]$User,
    [string]$UserLocusRoot
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

. (Join-Path $PSScriptRoot 'internal/locus-paths.ps1')

if (-not $User -and -not [string]::IsNullOrWhiteSpace($UserLocusRoot)) {
    throw '-UserLocusRoot requires -User'
}

function Assert-OrdinaryDirectory {
    param([Parameter(Mandatory)][string]$Path)

    if (-not (Test-Path -LiteralPath $Path)) {
        return
    }
    $item = Get-Item -LiteralPath $Path -Force
    if (-not $item.PSIsContainer -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw "cleanup target must be an ordinary directory: $Path"
    }
}

function Remove-WorkspaceDirectory {
    param([Parameter(Mandatory)][string]$Path)

    if (-not (Test-Path -LiteralPath $Path)) {
        return
    }
    Assert-OrdinaryDirectory $Path
    $reparsePoint = Get-ChildItem -LiteralPath $Path -Force -Recurse |
        Where-Object { ($_.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0 } |
        Select-Object -First 1
    if ($null -ne $reparsePoint) {
        throw "refusing to clean a directory containing a symbolic link or junction: $($reparsePoint.FullName)"
    }
    Remove-Item -LiteralPath $Path -Recurse -Force
    Write-Output "Removed $Path"
}

$RepositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
if ($User) {
    $LocusRoot = Resolve-LocusUserRoot -Override $UserLocusRoot
    $BinaryRoot = Join-Path $LocusRoot 'bin'
    Assert-OrdinaryDirectory $BinaryRoot
    foreach ($fileName in 'locus-scope', 'locus-scope.exe', 'locus-pkg', 'locus-pkg.exe') {
        $target = Join-Path $BinaryRoot $fileName
        if (Test-Path -LiteralPath $target -PathType Leaf) {
            $item = Get-Item -LiteralPath $target -Force
            if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
                throw "refusing to remove symbolic link: $target"
            }
            Remove-Item -LiteralPath $target -Force
            Write-Output "Removed $target"
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

$TempRoot = Join-Path $RepositoryRoot 'temp'
Assert-OrdinaryDirectory $TempRoot
Remove-WorkspaceDirectory (Join-Path $TempRoot 'build')
Remove-WorkspaceDirectory (Join-Path $TempRoot 'local')
