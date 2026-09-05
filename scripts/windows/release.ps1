[CmdletBinding()]
param(
    [string]$Version,
    [string]$IsccPath
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$RepositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$VersionFile = Join-Path $RepositoryRoot 'VERSION'
$ProjectVersion = (Get-Content -LiteralPath $VersionFile -Raw).Trim()
if ([string]::IsNullOrWhiteSpace($Version)) {
    $Version = $ProjectVersion
}
elseif ($Version -ne $ProjectVersion) {
    throw "release version $Version does not match VERSION ($ProjectVersion)"
}
if ($Version -notmatch '^[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$') {
    throw "VERSION contains an invalid semantic version: $Version"
}

$ReleaseRoot = Join-Path $RepositoryRoot 'temp\release'
$TempRoot = Join-Path $RepositoryRoot 'temp'
$ReleaseStageRoot = Join-Path $TempRoot 'release-stage'
$WindowsStageRoot = Join-Path $ReleaseStageRoot 'windows-amd64'
$WindowsReleaseRoot = Join-Path $ReleaseRoot 'windows-amd64'
$NpmRoot = Join-Path $ReleaseRoot 'npm'
$NpmStageRoot = Join-Path $NpmRoot 'stage'
$NpmTarballRoot = $NpmRoot
$InstallerScript = Join-Path $RepositoryRoot 'packaging\windows\locus.iss'
$LicensePath = Join-Path $RepositoryRoot 'LICENSE'
$InnoManagerPath = Join-Path $PSScriptRoot 'inno-setup.ps1'
$LdFlags = "-X locus-scope/internal/buildinfo.Version=$Version"

$PlatformPackages = @(
    [pscustomobject]@{ Directory = 'locus-scope-win32-x64'; Name = '@sundw/locus-scope-win32-x64'; NpmOS = 'win32'; NpmCPU = 'x64'; GoOS = 'windows'; GoArch = 'amd64'; Binary = 'locus-scope-node-host.exe' },
    [pscustomobject]@{ Directory = 'locus-scope-linux-x64'; Name = '@sundw/locus-scope-linux-x64'; NpmOS = 'linux'; NpmCPU = 'x64'; GoOS = 'linux'; GoArch = 'amd64'; Binary = 'locus-scope-node-host' },
    [pscustomobject]@{ Directory = 'locus-scope-linux-arm64'; Name = '@sundw/locus-scope-linux-arm64'; NpmOS = 'linux'; NpmCPU = 'arm64'; GoOS = 'linux'; GoArch = 'arm64'; Binary = 'locus-scope-node-host' },
    [pscustomobject]@{ Directory = 'locus-scope-darwin-x64'; Name = '@sundw/locus-scope-darwin-x64'; NpmOS = 'darwin'; NpmCPU = 'x64'; GoOS = 'darwin'; GoArch = 'amd64'; Binary = 'locus-scope-node-host' },
    [pscustomobject]@{ Directory = 'locus-scope-darwin-arm64'; Name = '@sundw/locus-scope-darwin-arm64'; NpmOS = 'darwin'; NpmCPU = 'arm64'; GoOS = 'darwin'; GoArch = 'arm64'; Binary = 'locus-scope-node-host' }
)

function Assert-File {
    param([Parameter(Mandatory)][string]$Path)

    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        throw "required release input is missing: $Path"
    }
}
function Remove-OwnedDirectory {
    param([Parameter(Mandatory)][string]$Path)

    if (-not (Test-Path -LiteralPath $Path)) {
        return
    }
    $item = Get-Item -LiteralPath $Path -Force
    if (-not $item.PSIsContainer -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw "release directory must be an ordinary directory: $Path"
    }
    $reparsePoint = Get-ChildItem -LiteralPath $Path -Force -Recurse |
        Where-Object { ($_.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0 } |
        Select-Object -First 1
    if ($null -ne $reparsePoint) {
        throw "refusing to remove release directory containing a symbolic link or junction: $($reparsePoint.FullName)"
    }
    Remove-Item -LiteralPath $Path -Recurse -Force
}


function Resolve-Iscc {
    if (-not [string]::IsNullOrWhiteSpace($IsccPath)) {
        $resolved = (Resolve-Path -LiteralPath $IsccPath).Path
        Assert-File $resolved
        return $resolved
    }
    if (-not [string]::IsNullOrWhiteSpace($env:ISCC_PATH)) {
        $resolved = (Resolve-Path -LiteralPath $env:ISCC_PATH).Path
        Assert-File $resolved
        return $resolved
    }

    Assert-File $InnoManagerPath
    $managedPath = & $InnoManagerPath path
    if ([string]::IsNullOrWhiteSpace($managedPath)) {
        throw 'managed Inno Setup is unavailable; run pnpm run inno:install'
    }
    return (Resolve-Path -LiteralPath $managedPath).Path
}

function Copy-PackageSource {
    param(
        [Parameter(Mandatory)][string]$Source,
        [Parameter(Mandatory)][string]$Destination
    )

    if (-not (Test-Path -LiteralPath $Source -PathType Container)) {
        throw "package source is missing: $Source"
    }
    $sourceItem = Get-Item -LiteralPath $Source -Force
    if (($sourceItem.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw "package source must not be a symbolic link or junction: $Source"
    }
    New-Item -ItemType Directory -Force -Path $Destination | Out-Null
    foreach ($item in Get-ChildItem -LiteralPath $Source -Force) {
        if ($item.Name -eq 'node_modules') {
            continue
        }
        if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw "package source must not contain a symbolic link or junction: $($item.FullName)"
        }
        if ($item.PSIsContainer) {
            $reparsePoint = Get-ChildItem -LiteralPath $item.FullName -Force -Recurse |
                Where-Object { ($_.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0 } |
                Select-Object -First 1
            if ($null -ne $reparsePoint) {
                throw "package source must not contain a symbolic link or junction: $($reparsePoint.FullName)"
            }
        }
        Copy-Item -LiteralPath $item.FullName -Destination $Destination -Recurse -Force
    }
    Copy-Item -LiteralPath $LicensePath -Destination (Join-Path $Destination 'LICENSE') -Force
}

function Set-StagedPackageVersion {
    param(
        [Parameter(Mandatory)][string]$PackageRoot,
        [Parameter(Mandatory)][string]$ExpectedName,
        [string]$ExpectedOS,
        [string]$ExpectedCPU,
        [string]$ExpectedFile,
        [switch]$SynchronizeOptionalDependencies
    )

    $manifestPath = Join-Path $PackageRoot 'package.json'
    Assert-File $manifestPath
    $manifest = Get-Content -LiteralPath $manifestPath -Raw | ConvertFrom-Json
    if ($manifest.name -ne $ExpectedName) {
        throw "unexpected package name '$($manifest.name)' in $manifestPath; expected '$ExpectedName'"
    }
    if ($manifest.PSObject.Properties.Name -contains 'version') {
        throw "$ExpectedName source manifest must not declare version; VERSION is the release version source"
    }
    if ($manifest.private -ne $true) {
        throw "$ExpectedName source manifest must be private"
    }
    if (-not [string]::IsNullOrWhiteSpace($ExpectedOS)) {
        if (@($manifest.os).Count -ne 1 -or $manifest.os[0] -ne $ExpectedOS -or
            @($manifest.cpu).Count -ne 1 -or $manifest.cpu[0] -ne $ExpectedCPU -or
            @($manifest.files).Count -ne 1 -or $manifest.files[0] -ne $ExpectedFile) {
            throw "$ExpectedName platform constraints or packaged binary path are invalid"
        }
        if ($manifest.PSObject.Properties.Name -contains 'bin') {
            throw "$ExpectedName source manifest must not declare bin; release staging owns executable metadata"
        }
        if ($ExpectedOS -ne 'win32') {
            $manifest | Add-Member -NotePropertyName bin -NotePropertyValue ([pscustomobject]@{ 'locus-scope-node-host' = $ExpectedFile })
        }
    }
    $manifest.PSObject.Properties.Remove('private')
    $manifest | Add-Member -NotePropertyName version -NotePropertyValue $Version
    if ($SynchronizeOptionalDependencies) {
        if ($null -eq $manifest.optionalDependencies) {
            throw "$ExpectedName must declare platform optionalDependencies"
        }
        foreach ($dependency in $manifest.optionalDependencies.PSObject.Properties) {
            $dependency.Value = $Version
        }
        $expectedDependencies = @($PlatformPackages | ForEach-Object { $_.Name } | Sort-Object)
        $actualDependencies = @($manifest.optionalDependencies.PSObject.Properties.Name | Sort-Object)
        if (($expectedDependencies -join "`n") -ne ($actualDependencies -join "`n")) {
            throw "$ExpectedName optionalDependencies do not match the five platform packages"
        }
    }
    $content = ($manifest | ConvertTo-Json -Depth 20) + "`n"
    [IO.File]::WriteAllText($manifestPath, $content, (New-Object Text.UTF8Encoding($false)))
}

function Invoke-PackagePack {
    param([Parameter(Mandatory)][string]$PackageRoot)

    & pnpm --dir $PackageRoot pack --pack-destination $NpmTarballRoot
    if ($LASTEXITCODE -ne 0) {
        throw "pnpm pack failed for $PackageRoot with code $LASTEXITCODE"
    }
}

function Write-Checksums {
    param(
        [Parameter(Mandatory)][string]$Directory,
        [Parameter(Mandatory)][string[]]$Artifacts
    )

    $lines = foreach ($artifact in $Artifacts | Sort-Object) {
        Assert-File $artifact
        $hash = (Get-FileHash -LiteralPath $artifact -Algorithm SHA256).Hash.ToLowerInvariant()
        "$hash *$([IO.Path]::GetFileName($artifact))"
    }
    [IO.File]::WriteAllText((Join-Path $Directory 'SHA256SUMS'), (($lines -join "`n") + "`n"), (New-Object Text.UTF8Encoding($false)))
}

Assert-File $InstallerScript
Assert-File $LicensePath
Get-Command pnpm -ErrorAction Stop | Out-Null
$CompilerPath = Resolve-Iscc
foreach ($path in $TempRoot, $ReleaseStageRoot) {
    if (-not (Test-Path -LiteralPath $path)) {
        continue
    }
    $item = Get-Item -LiteralPath $path -Force
    if (-not $item.PSIsContainer -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw "release path must be an ordinary directory: $path"
    }
}
Remove-OwnedDirectory $ReleaseRoot
Remove-OwnedDirectory $WindowsStageRoot
$StageBinRoot = Join-Path $WindowsStageRoot 'bin'
$StageLicenseRoot = Join-Path $WindowsStageRoot 'licenses'
New-Item -ItemType Directory -Force -Path $StageBinRoot, $StageLicenseRoot, $WindowsReleaseRoot, $NpmStageRoot, $NpmTarballRoot | Out-Null

$previousGoOS = $env:GOOS
$previousGoArch = $env:GOARCH
$previousCgoEnabled = $env:CGO_ENABLED
try {
    $env:CGO_ENABLED = '0'
    $env:GOOS = 'windows'
    $env:GOARCH = 'amd64'
    & node (Join-Path $RepositoryRoot 'scripts\build.mjs')
    if ($LASTEXITCODE -ne 0) {
        throw "node scripts/build.mjs exited with code $LASTEXITCODE"
    }
    $artifactRoot = Join-Path $RepositoryRoot 'temp\build\windows-amd64'
    foreach ($fileName in 'locus-pkg.exe', 'locus-scope.exe') {
        $source = Join-Path $artifactRoot $fileName
        Assert-File $source
        Copy-Item -LiteralPath $source -Destination (Join-Path $StageBinRoot $fileName)
    }
    Copy-Item -LiteralPath $LicensePath -Destination (Join-Path $StageLicenseRoot 'locus-license.txt')

    foreach ($platformPackage in $PlatformPackages) {
        $sourceRoot = Join-Path $RepositoryRoot (Join-Path 'packaging\npm' $platformPackage.Directory)
        $stageRoot = Join-Path $NpmStageRoot $platformPackage.Directory
        Copy-PackageSource -Source $sourceRoot -Destination $stageRoot
        Set-StagedPackageVersion -PackageRoot $stageRoot -ExpectedName $platformPackage.Name -ExpectedOS $platformPackage.NpmOS -ExpectedCPU $platformPackage.NpmCPU -ExpectedFile "bin/$($platformPackage.Binary)"
        $binaryRoot = Join-Path $stageRoot 'bin'
        New-Item -ItemType Directory -Force -Path $binaryRoot | Out-Null
        $binaryPath = Join-Path $binaryRoot $platformPackage.Binary
        $env:GOOS = $platformPackage.GoOS
        $env:GOARCH = $platformPackage.GoArch
        Push-Location $RepositoryRoot
        try {
            & go build -trimpath -ldflags $LdFlags -o $binaryPath ./cmd/locus-scope-node-host
            if ($LASTEXITCODE -ne 0) {
                throw "go build locus-scope-node-host for $($platformPackage.GoOS)/$($platformPackage.GoArch) exited with code $LASTEXITCODE"
            }
        }
        finally {
            Pop-Location
        }
        Assert-File $binaryPath
        Invoke-PackagePack -PackageRoot $stageRoot
    }

    $scopeSourceRoot = Join-Path $RepositoryRoot 'packaging\npm\locus-scope'
    $scopeStageRoot = Join-Path $NpmStageRoot 'locus-scope'
    Copy-PackageSource -Source $scopeSourceRoot -Destination $scopeStageRoot
    Set-StagedPackageVersion -PackageRoot $scopeStageRoot -ExpectedName '@sundw/locus-scope' -SynchronizeOptionalDependencies
    Invoke-PackagePack -PackageRoot $scopeStageRoot
}
finally {
    if ($null -eq $previousGoOS) {
        Remove-Item Env:GOOS -ErrorAction SilentlyContinue
    }
    else {
        $env:GOOS = $previousGoOS
    }
    if ($null -eq $previousGoArch) {
        Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
    }
    else {
        $env:GOARCH = $previousGoArch
    }
    if ($null -eq $previousCgoEnabled) {
        Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue
    }
    else {
        $env:CGO_ENABLED = $previousCgoEnabled
    }
}

$cliArchive = Join-Path $WindowsReleaseRoot 'locus-windows-amd64.zip'
$archiveInputs = @(
    (Join-Path $StageBinRoot 'locus-pkg.exe'),
    (Join-Path $StageBinRoot 'locus-scope.exe'),
    (Join-Path $StageLicenseRoot 'locus-license.txt')
)
Compress-Archive -LiteralPath $archiveInputs -DestinationPath $cliArchive -CompressionLevel Optimal
& $CompilerPath "/DAppVersion=$Version" "/DStageDir=$WindowsStageRoot" "/DOutputDir=$WindowsReleaseRoot" $InstallerScript
if ($LASTEXITCODE -ne 0) {
    throw "ISCC.exe exited with code $LASTEXITCODE"
}
$setupPath = Join-Path $WindowsReleaseRoot 'locus-setup-windows-amd64.exe'
Assert-File $setupPath
Write-Checksums -Directory $WindowsReleaseRoot -Artifacts @($cliArchive, $setupPath)

$npmTarballs = @(Get-ChildItem -LiteralPath $NpmTarballRoot -Filter '*.tgz' -File | Select-Object -ExpandProperty FullName)
if ($npmTarballs.Count -ne 6) {
    throw "expected six npm package tarballs, found $($npmTarballs.Count)"
}
& node (Join-Path $RepositoryRoot 'scripts\verify-npm-release.mjs') $Version
if ($LASTEXITCODE -ne 0) {
    throw "npm release verification exited with code $LASTEXITCODE"
}
Write-Checksums -Directory $NpmTarballRoot -Artifacts $npmTarballs

Write-Output "Standalone release artifacts: $WindowsReleaseRoot"
Get-ChildItem -LiteralPath $WindowsReleaseRoot -File | Sort-Object Name | Select-Object Name, Length
Write-Output "npm package artifacts: $NpmTarballRoot"
Get-ChildItem -LiteralPath $NpmTarballRoot -File | Sort-Object Name | Select-Object Name, Length
