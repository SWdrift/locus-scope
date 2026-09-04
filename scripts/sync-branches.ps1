[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$RepositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$MergeHeadPath = $null

function Invoke-Git {
    param(
        [Parameter(Mandatory)]
        [string[]]$GitArguments
    )

    & git @GitArguments
    if ($LASTEXITCODE -ne 0) {
        throw "git $($GitArguments -join ' ') exited with code $LASTEXITCODE"
    }
}

Push-Location $RepositoryRoot
try {
    $Status = @(Invoke-Git -GitArguments @('status', '--porcelain=v1'))
    if ($Status.Count -ne 0) {
        throw 'Working tree must be clean before synchronizing branches.'
    }

    $MergeHeadPath = (Invoke-Git -GitArguments @('rev-parse', '--git-path', 'MERGE_HEAD') | Out-String).Trim()

    Invoke-Git -GitArguments @('switch', 'dev')
    Invoke-Git -GitArguments @('fetch', 'gitee', 'main')
    Invoke-Git -GitArguments @('fetch', 'github', 'master')

    Invoke-Git -GitArguments @('switch', 'main')
    Invoke-Git -GitArguments @('merge', '--ff-only', 'gitee/main')
    Invoke-Git -GitArguments @('merge', '--no-ff', '--no-edit', 'dev')

    Invoke-Git -GitArguments @('switch', 'master')
    Invoke-Git -GitArguments @('merge', '--ff-only', 'github/master')
    Invoke-Git -GitArguments @('merge', '--no-ff', '--no-edit', 'dev')

    Invoke-Git -GitArguments @('push', 'gitee', 'main:main')
    Invoke-Git -GitArguments @('push', 'github', 'master:master')
    Invoke-Git -GitArguments @('switch', 'dev')

    Write-Output 'Synchronized dev into gitee/main and github/master; current branch is dev.'
}
catch {
    $Failure = $_

    if ($MergeHeadPath -and (Test-Path -LiteralPath $MergeHeadPath)) {
        & git merge --abort
        if ($LASTEXITCODE -ne 0) {
            Write-Warning 'Failed to abort the in-progress merge.'
        }
    }

    $CurrentBranch = (& git branch --show-current | Out-String).Trim()
    if ($LASTEXITCODE -eq 0 -and $CurrentBranch -and $CurrentBranch -ne 'dev') {
        & git switch dev
        if ($LASTEXITCODE -ne 0) {
            Write-Warning 'Failed to return to dev; resolve the repository state manually.'
        }
    }

    throw $Failure
}
finally {
    Pop-Location
}
