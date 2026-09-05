[CmdletBinding()]
param(
    [Parameter(Position = 0)]
    [ValidateSet('e2e', 'all')]
    [string]$Suite = 'all'
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$RepositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$RequiredNodeFiles = @(
    (Join-Path $RepositoryRoot 'node_modules\verdaccio\package.json'),
    (Join-Path $RepositoryRoot 'node_modules\remark-cli\package.json'),
    (Join-Path $RepositoryRoot 'node_modules\remark-validate-links\package.json')
)
foreach ($requiredFile in $RequiredNodeFiles) {
    if (-not (Test-Path -LiteralPath $requiredFile -PathType Leaf)) {
        throw 'root Node dependencies are not installed; run: pwsh -File scripts/npm-registry.ps1 install'
    }
}

Push-Location $RepositoryRoot
try {
    if ($Suite -eq 'e2e') {
        & go test ./test/e2e '-count=1'
        if ($LASTEXITCODE -ne 0) {
            throw "go test ./test/e2e exited with code $LASTEXITCODE"
        }
        return
    }

    & go test ./... '-count=1'
    if ($LASTEXITCODE -ne 0) {
        throw "go test ./... exited with code $LASTEXITCODE"
    }

    & pnpm run test:node
    if ($LASTEXITCODE -ne 0) {
        throw "pnpm run test:node exited with code $LASTEXITCODE"
    }
}
finally {
    Pop-Location
}
