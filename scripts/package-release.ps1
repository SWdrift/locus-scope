[CmdletBinding()]
param(
    [string]$Version,
    [string]$ZotBinary,
    [string]$IsccPath
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$RepositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
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
$Platform = 'windows-amd64'
$StageRoot = Join-Path $RepositoryRoot "temp\release-stage\$Platform"
$ReleaseRoot = Join-Path $RepositoryRoot "temp\release\$Platform"
$InstallerScript = Join-Path $RepositoryRoot 'installer\windows\locus.iss'
$ZotLicense = Join-Path $RepositoryRoot 'installer\windows\assets\zot-license.txt'
$ZotManager = Join-Path $PSScriptRoot 'internal\locus-zot-user.ps1'
$ZotRelease = Import-PowerShellDataFile (Join-Path $PSScriptRoot 'internal\zot-release.psd1')

function Resolve-Iscc {
    if (-not [string]::IsNullOrWhiteSpace($IsccPath)) {
        return (Resolve-Path -LiteralPath $IsccPath).Path
    }
    if (-not [string]::IsNullOrWhiteSpace($env:ISCC_PATH)) {
        return (Resolve-Path -LiteralPath $env:ISCC_PATH).Path
    }

    $command = Get-Command ISCC.exe -ErrorAction SilentlyContinue
    if ($null -ne $command) {
        return $command.Source
    }

    $candidates = @(
        (Join-Path ${env:ProgramFiles(x86)} 'Inno Setup 6\ISCC.exe'),
        (Join-Path $env:ProgramFiles 'Inno Setup 6\ISCC.exe'),
        (Join-Path $env:LOCALAPPDATA 'Programs\Inno Setup 6\ISCC.exe')
    )
    foreach ($candidate in $candidates) {
        if (-not [string]::IsNullOrWhiteSpace($candidate) -and (Test-Path -LiteralPath $candidate -PathType Leaf)) {
            return (Resolve-Path -LiteralPath $candidate).Path
        }
    }
    throw 'Inno Setup 6 compiler ISCC.exe was not found. Install Inno Setup 6, pass -IsccPath, or set ISCC_PATH.'
}

function Assert-File {
    param([Parameter(Mandatory)][string]$Path)

    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        throw "required release input is missing: $Path"
    }
}

$CompilerPath = Resolve-Iscc
foreach ($requiredPath in $InstallerScript, $ZotLicense, $ZotManager, (Join-Path $RepositoryRoot 'LICENSE')) {
    Assert-File $requiredPath
}

if (Test-Path -LiteralPath $StageRoot) {
    Remove-Item -LiteralPath $StageRoot -Recurse -Force
}
if (Test-Path -LiteralPath $ReleaseRoot) {
    Remove-Item -LiteralPath $ReleaseRoot -Recurse -Force
}
$StageBinRoot = Join-Path $StageRoot 'bin'
$StageZotRoot = Join-Path $StageRoot 'zot'
$StageLibexecRoot = Join-Path $StageRoot 'libexec'
$StageLicenseRoot = Join-Path $StageRoot 'licenses'
New-Item -ItemType Directory -Force -Path $StageBinRoot, $StageZotRoot, $StageLibexecRoot, $StageLicenseRoot, $ReleaseRoot | Out-Null

$previousGoOS = $env:GOOS
$previousGoArch = $env:GOARCH
try {
    $env:GOOS = 'windows'
    $env:GOARCH = 'amd64'
    $buildResult = & (Join-Path $PSScriptRoot 'build.ps1') -PassThru
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
}

foreach ($fileName in 'locus-pkg.exe', 'locus-scope.exe') {
    $source = Join-Path $buildResult.ArtifactRoot $fileName
    Assert-File $source
    Copy-Item -LiteralPath $source -Destination (Join-Path $StageBinRoot $fileName)
}
Copy-Item -LiteralPath $ZotManager -Destination (Join-Path $StageLibexecRoot 'locus-zot-user.ps1')
Copy-Item -LiteralPath (Join-Path $RepositoryRoot 'LICENSE') -Destination (Join-Path $StageLicenseRoot 'locus-license.txt')
Copy-Item -LiteralPath $ZotLicense -Destination (Join-Path $StageLicenseRoot 'zot-license.txt')

$stagedZot = Join-Path $StageZotRoot 'zot.exe'
if ([string]::IsNullOrWhiteSpace($ZotBinary)) {
    Invoke-WebRequest -UseBasicParsing -Uri "$($ZotRelease.ReleaseBase)/$($ZotRelease.Asset)" -OutFile $stagedZot
}
else {
    $resolvedZotBinary = (Resolve-Path -LiteralPath $ZotBinary).Path
    Copy-Item -LiteralPath $resolvedZotBinary -Destination $stagedZot
}
$actualZotHash = (Get-FileHash -LiteralPath $stagedZot -Algorithm SHA256).Hash.ToLowerInvariant()
if ($actualZotHash -ne $ZotRelease.Sha256) {
    throw "Zot binary hash mismatch: expected $($ZotRelease.Sha256), got $actualZotHash"
}

$cliArchive = Join-Path $ReleaseRoot 'locus-windows-amd64.zip'
$archiveInputs = @(
    (Join-Path $StageBinRoot 'locus-pkg.exe'),
    (Join-Path $StageBinRoot 'locus-scope.exe'),
    (Join-Path $StageLicenseRoot 'locus-license.txt')
)
Compress-Archive -LiteralPath $archiveInputs -DestinationPath $cliArchive -CompressionLevel Optimal
Copy-Item -LiteralPath $stagedZot -Destination (Join-Path $ReleaseRoot 'zot-windows-amd64.exe')

& $CompilerPath "/DAppVersion=$Version" "/DStageDir=$StageRoot" "/DOutputDir=$ReleaseRoot" $InstallerScript
if ($LASTEXITCODE -ne 0) {
    throw "ISCC.exe exited with code $LASTEXITCODE"
}

$setupPath = Join-Path $ReleaseRoot 'locus-setup-windows-amd64.exe'
Assert-File $setupPath
$releaseArtifacts = @(
    $cliArchive,
    (Join-Path $ReleaseRoot 'zot-windows-amd64.exe'),
    $setupPath
)
$checksumLines = foreach ($artifact in $releaseArtifacts) {
    $hash = (Get-FileHash -LiteralPath $artifact -Algorithm SHA256).Hash.ToLowerInvariant()
    "$hash *$([IO.Path]::GetFileName($artifact))"
}
$checksumContent = ($checksumLines -join "`n") + "`n"
[IO.File]::WriteAllText((Join-Path $ReleaseRoot 'SHA256SUMS'), $checksumContent, (New-Object Text.UTF8Encoding($false)))

Write-Output "Release artifacts: $ReleaseRoot"
Get-ChildItem -LiteralPath $ReleaseRoot -File | Sort-Object Name | Select-Object Name, Length
