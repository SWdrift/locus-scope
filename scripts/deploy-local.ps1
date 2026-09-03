[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$RepositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$BuildScript = Join-Path $PSScriptRoot 'build.ps1'

$BuildResult = & $BuildScript -PassThru
$ArtifactRoot = $BuildResult.ArtifactRoot
$DeploymentRoot = Join-Path $RepositoryRoot 'temp\local\bin'
if (Test-Path -LiteralPath $DeploymentRoot) {
    Remove-Item -LiteralPath $DeploymentRoot -Recurse -Force
}
New-Item -ItemType Directory -Force -Path $DeploymentRoot | Out-Null

foreach ($Name in 'locus-scope', 'locus-pkg') {
    $FileName = $Name + $BuildResult.Extension
    $SourcePath = Join-Path $ArtifactRoot $FileName
    if (-not (Test-Path -LiteralPath $SourcePath -PathType Leaf)) {
        throw "build artifact is missing: $SourcePath"
    }
    Copy-Item -LiteralPath $SourcePath -Destination (Join-Path $DeploymentRoot $FileName)
}

Write-Output "Deployed local CLI binaries to $DeploymentRoot"
