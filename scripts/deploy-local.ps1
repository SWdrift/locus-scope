[CmdletBinding()]
param(
    [switch]$User,
    [switch]$WithZot,
    [string]$UserLocusRoot,
    [string]$ZotBinary
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

. (Join-Path $PSScriptRoot 'internal/locus-paths.ps1')

if (-not $User -and -not [string]::IsNullOrWhiteSpace($UserLocusRoot)) {
    throw '-UserLocusRoot requires -User'
}
if (-not $WithZot -and -not [string]::IsNullOrWhiteSpace($ZotBinary)) {
    throw '-ZotBinary requires -WithZot'
}

$RepositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$BuildScript = Join-Path $PSScriptRoot 'build.ps1'
$BuildResult = & $BuildScript -PassThru
$ArtifactRoot = $BuildResult.ArtifactRoot

if ($User) {
    $LocusRoot = Resolve-LocusUserRoot -Override $UserLocusRoot
    $DeploymentRoot = Join-Path $LocusRoot 'bin'
    New-Item -ItemType Directory -Force -Path $DeploymentRoot | Out-Null
}
else {
    $DeploymentRoot = Join-Path $RepositoryRoot 'temp/local/bin'
    if (Test-Path -LiteralPath $DeploymentRoot) {
        Remove-Item -LiteralPath $DeploymentRoot -Recurse -Force
    }
    New-Item -ItemType Directory -Force -Path $DeploymentRoot | Out-Null
}

foreach ($Name in 'locus-scope', 'locus-pkg') {
    $FileName = $Name + $BuildResult.Extension
    $SourcePath = Join-Path $ArtifactRoot $FileName
    if (-not (Test-Path -LiteralPath $SourcePath -PathType Leaf)) {
        throw "build artifact is missing: $SourcePath"
    }
    $DestinationPath = Join-Path $DeploymentRoot $FileName
    if ($User) {
        $TemporaryPath = "$DestinationPath.deploy-$PID"
        try {
            Copy-Item -LiteralPath $SourcePath -Destination $TemporaryPath -Force
            Move-Item -LiteralPath $TemporaryPath -Destination $DestinationPath -Force
        }
        finally {
            Remove-Item -LiteralPath $TemporaryPath -Force -ErrorAction SilentlyContinue
        }
    }
    else {
        Copy-Item -LiteralPath $SourcePath -Destination $DestinationPath
    }
}

Write-Output "Deployed local CLI binaries to $DeploymentRoot"

if ($WithZot) {
    $ZotParameters = @{ Action = 'install' }
    if ($User) {
        $ZotParameters.User = $true
        $ZotParameters.UserLocusRoot = $LocusRoot
    }
    if (-not [string]::IsNullOrWhiteSpace($ZotBinary)) {
        $ZotParameters.InstallSource = $ZotBinary
    }
    & (Join-Path $PSScriptRoot 'zot.ps1') @ZotParameters
}
