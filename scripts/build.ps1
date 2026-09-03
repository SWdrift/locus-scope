[CmdletBinding()]
param(
    [switch]$PassThru
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$RepositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path

Push-Location $RepositoryRoot
try {
    $GoOS = (& go env GOOS | Out-String).Trim()
    if ($LASTEXITCODE -ne 0) {
        throw "go env GOOS exited with code $LASTEXITCODE"
    }
    $GoArch = (& go env GOARCH | Out-String).Trim()
    if ($LASTEXITCODE -ne 0) {
        throw "go env GOARCH exited with code $LASTEXITCODE"
    }

    $ArtifactRoot = Join-Path $RepositoryRoot "temp\build\$GoOS-$GoArch"
    if (Test-Path -LiteralPath $ArtifactRoot) {
        Remove-Item -LiteralPath $ArtifactRoot -Recurse -Force
    }
    New-Item -ItemType Directory -Force -Path $ArtifactRoot | Out-Null

    $Extension = if ($GoOS -eq 'windows') { '.exe' } else { '' }
    $Commands = @(
        @{ Name = 'locus-scope'; Package = './cmd/locus-scope' },
        @{ Name = 'locus-pkg'; Package = './cmd/locus-pkg' }
    )

    foreach ($Command in $Commands) {
        $OutputPath = Join-Path $ArtifactRoot ($Command.Name + $Extension)
        & go build -trimpath -o $OutputPath $Command.Package
        if ($LASTEXITCODE -ne 0) {
            throw "go build $($Command.Package) exited with code $LASTEXITCODE"
        }
        if (-not $PassThru) {
            Write-Output "Built $OutputPath"
        }
    }

    if ($PassThru) {
        [pscustomobject]@{
            ArtifactRoot = $ArtifactRoot
            Extension = $Extension
        }
    }
}
finally {
    Pop-Location
}
