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
        throw "deployment path must be an ordinary directory: $Path"
    }
}

function Assert-OrdinaryFileOrMissing {
    param([Parameter(Mandatory)][string]$Path)

    if (-not (Test-Path -LiteralPath $Path)) {
        return
    }
    $item = Get-Item -LiteralPath $Path -Force
    if ($item.PSIsContainer -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw "deployment file must be an ordinary file: $Path"
    }
}

$RepositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$BuildResult = & (Join-Path $PSScriptRoot 'local-build.ps1') -PassThru
$ArtifactRoot = $BuildResult.ArtifactRoot

if ($User) {
    $LocusRoot = Resolve-LocusUserRoot -Override $UserLocusRoot
    $DeploymentRoot = Join-Path $LocusRoot 'bin'
    Assert-OrdinaryDirectory $DeploymentRoot
    New-Item -ItemType Directory -Force -Path $DeploymentRoot | Out-Null
}
else {
    $TempRoot = Join-Path $RepositoryRoot 'temp'
    $LocalRoot = Join-Path $TempRoot 'local'
    $DeploymentRoot = Join-Path $LocalRoot 'bin'
    Assert-OrdinaryDirectory $TempRoot
    Assert-OrdinaryDirectory $LocalRoot
    Assert-OrdinaryDirectory $DeploymentRoot
    if (Test-Path -LiteralPath $DeploymentRoot) {
        $reparsePoint = Get-ChildItem -LiteralPath $DeploymentRoot -Force -Recurse |
            Where-Object { ($_.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0 } |
            Select-Object -First 1
        if ($null -ne $reparsePoint) {
            throw "refusing to replace deployment containing a symbolic link or junction: $($reparsePoint.FullName)"
        }
        Remove-Item -LiteralPath $DeploymentRoot -Recurse -Force
    }
    New-Item -ItemType Directory -Force -Path $DeploymentRoot | Out-Null
}

foreach ($name in 'locus-scope', 'locus-pkg') {
    $fileName = $name + $BuildResult.Extension
    $sourcePath = Join-Path $ArtifactRoot $fileName
    if (-not (Test-Path -LiteralPath $sourcePath -PathType Leaf)) {
        throw "build artifact is missing: $sourcePath"
    }
    Assert-OrdinaryFileOrMissing $sourcePath
    $destinationPath = Join-Path $DeploymentRoot $fileName
    Assert-OrdinaryFileOrMissing $destinationPath
    if (-not $User) {
        Copy-Item -LiteralPath $sourcePath -Destination $destinationPath
        continue
    }

    $temporaryPath = "$destinationPath.deploy-$PID"
    Assert-OrdinaryFileOrMissing $temporaryPath
    try {
        Copy-Item -LiteralPath $sourcePath -Destination $temporaryPath -Force
        Move-Item -LiteralPath $temporaryPath -Destination $destinationPath -Force
    }
    finally {
        Remove-Item -LiteralPath $temporaryPath -Force -ErrorAction SilentlyContinue
    }
}

Write-Output "Deployed local CLI binaries to $DeploymentRoot"
