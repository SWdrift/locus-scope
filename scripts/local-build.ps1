[CmdletBinding()]
param(
    [switch]$PassThru
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$RepositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$Version = (Get-Content -LiteralPath (Join-Path $RepositoryRoot 'VERSION') -Raw).Trim()
if ($Version -notmatch '^[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$') {
    throw "VERSION contains an invalid semantic version: $Version"
}
$LdFlags = "-X locus-scope/internal/buildinfo.Version=$Version"
function Assert-OrdinaryDirectory {
    param([Parameter(Mandatory)][string]$Path)

    if (-not (Test-Path -LiteralPath $Path)) {
        return
    }
    $item = Get-Item -LiteralPath $Path -Force
    if (-not $item.PSIsContainer -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw "build path must be an ordinary directory: $Path"
    }
}


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

    $TempRoot = Join-Path $RepositoryRoot 'temp'
    $BuildRoot = Join-Path $TempRoot 'build'
    Assert-OrdinaryDirectory $TempRoot
    Assert-OrdinaryDirectory $BuildRoot
    New-Item -ItemType Directory -Force -Path $BuildRoot | Out-Null

    $ArtifactRoot = Join-Path $BuildRoot "$GoOS-$GoArch"
    Assert-OrdinaryDirectory $ArtifactRoot
    if (Test-Path -LiteralPath $ArtifactRoot) {
        $reparsePoint = Get-ChildItem -LiteralPath $ArtifactRoot -Force -Recurse |
            Where-Object { ($_.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0 } |
            Select-Object -First 1
        if ($null -ne $reparsePoint) {
            throw "refusing to replace build output containing a symbolic link or junction: $($reparsePoint.FullName)"
        }
        Remove-Item -LiteralPath $ArtifactRoot -Recurse -Force
    }
    New-Item -ItemType Directory -Path $ArtifactRoot | Out-Null

    $Extension = if ($GoOS -eq 'windows') { '.exe' } else { '' }
    $Commands = @(
        @{ Name = 'locus-scope'; Package = './cmd/locus-scope' },
        @{ Name = 'locus-pkg'; Package = './cmd/locus-pkg' }
    )

    foreach ($Command in $Commands) {
        $OutputPath = Join-Path $ArtifactRoot ($Command.Name + $Extension)
        & go build -trimpath -ldflags $LdFlags -o $OutputPath $Command.Package
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
            Version = $Version
        }
    }
}
finally {
    Pop-Location
}
