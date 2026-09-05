[CmdletBinding()]
param(
    [Parameter(Position = 0)]
    [ValidateSet('install', 'status', 'path', 'reset')]
    [string]$Action = 'status',
    [switch]$Force
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

if (-not $IsWindows) {
    throw 'Inno Setup management is supported only on Windows'
}

$RepositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$Version = '6.7.3'
$InstallerSha256 = '9c73c3bae7ed48d44112a0f48e66742c00090bdb5bef71d9d3c056c66e97b732'
$DownloadUri = 'https://github.com/jrsoftware/issrc/releases/download/is-6_7_3/innosetup-6.7.3.exe'
$ToolsRoot = Join-Path $RepositoryRoot 'temp\tools'
$InstallRoot = Join-Path $ToolsRoot "inno-$Version"
$InstallerPath = Join-Path $ToolsRoot "innosetup-$Version.exe"
$DownloadPath = $InstallerPath + '.download'
$CompilerPath = Join-Path $InstallRoot 'ISCC.exe'
$ManifestPath = Join-Path $InstallRoot '.locus-tool.json'
$InstallLogPath = Join-Path $ToolsRoot "inno-$Version-install.log"

function Assert-OrdinaryDirectory {
    param([Parameter(Mandatory)][string]$Path)

    if (-not (Test-Path -LiteralPath $Path)) {
        return
    }
    $item = Get-Item -LiteralPath $Path -Force
    if (-not $item.PSIsContainer -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw "Inno Setup path must be an ordinary directory: $Path"
    }
}

function Assert-OrdinaryFile {
    param([Parameter(Mandatory)][string]$Path)

    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        throw "required Inno Setup file is missing: $Path"
    }
    $item = Get-Item -LiteralPath $Path -Force
    if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw "Inno Setup file must not be a symbolic link: $Path"
    }
}

function Remove-OwnedDirectory {
    param([Parameter(Mandatory)][string]$Path)

    if (-not (Test-Path -LiteralPath $Path)) {
        return
    }
    Assert-OrdinaryDirectory $Path
    $reparsePoint = Get-ChildItem -LiteralPath $Path -Force -Recurse |
        Where-Object { ($_.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0 } |
        Select-Object -First 1
    if ($null -ne $reparsePoint) {
        throw "refusing to remove Inno Setup directory containing a symbolic link or junction: $($reparsePoint.FullName)"
    }
    Remove-Item -LiteralPath $Path -Recurse -Force
}

function Assert-Installer {
    Assert-OrdinaryFile $InstallerPath
    $actualHash = (Get-FileHash -LiteralPath $InstallerPath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actualHash -ne $InstallerSha256) {
        throw "Inno Setup installer SHA-256 mismatch at $InstallerPath"
    }
    $signature = Get-AuthenticodeSignature -LiteralPath $InstallerPath
    if ($signature.Status -ne [Management.Automation.SignatureStatus]::Valid) {
        throw "Inno Setup installer signature is not valid at $InstallerPath`: $($signature.Status)"
    }
}

function Assert-Installation {
    Assert-OrdinaryDirectory $ToolsRoot
    Assert-OrdinaryDirectory $InstallRoot
    Assert-OrdinaryFile $CompilerPath
    Assert-OrdinaryFile $ManifestPath

    try {
        $manifest = Get-Content -LiteralPath $ManifestPath -Raw | ConvertFrom-Json
    }
    catch {
        throw "invalid managed Inno Setup manifest: $ManifestPath"
    }
    if ($manifest.version -ne $Version -or $manifest.source -ne $DownloadUri -or $manifest.installerSha256 -ne $InstallerSha256) {
        throw "managed Inno Setup manifest does not describe version $Version"
    }
    $compilerHash = (Get-FileHash -LiteralPath $CompilerPath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($manifest.compilerSha256 -ne $compilerHash) {
        throw "managed ISCC.exe SHA-256 mismatch at $CompilerPath"
    }
}

function Get-ManagedCompilerPath {
    try {
        Assert-Installation
    }
    catch {
        throw "managed Inno Setup $Version is unavailable; run pnpm run inno:install: $($_.Exception.Message)"
    }
    return $CompilerPath
}


function Get-Installer {
    Assert-OrdinaryDirectory $ToolsRoot
    New-Item -ItemType Directory -Force -Path $ToolsRoot | Out-Null
    Assert-OrdinaryDirectory $ToolsRoot

    if (Test-Path -LiteralPath $InstallerPath) {
        Assert-Installer
        return
    }

    Remove-Item -LiteralPath $DownloadPath -Force -ErrorAction SilentlyContinue
    try {
        Invoke-WebRequest -UseBasicParsing -Uri $DownloadUri -OutFile $DownloadPath
        Move-Item -LiteralPath $DownloadPath -Destination $InstallerPath
        Assert-Installer
    }
    catch {
        Remove-Item -LiteralPath $DownloadPath, $InstallerPath -Force -ErrorAction SilentlyContinue
        throw
    }
}

function Install-InnoSetup {
    if (Test-Path -LiteralPath $InstallRoot) {
        try {
            Assert-Installation
            Write-Output "Inno Setup $Version is already installed at $InstallRoot"
            return
        }
        catch {
            throw "managed Inno Setup state is invalid; run pnpm run inno:reset before reinstalling: $($_.Exception.Message)"
        }
    }

    Get-Installer
    $arguments = @(
        '/PORTABLE=1',
        '/VERYSILENT',
        '/SUPPRESSMSGBOXES',
        '/NORESTART',
        '/SP-',
        '/NOICONS',
        ('/DIR="{0}"' -f $InstallRoot),
        ('/LOG="{0}"' -f $InstallLogPath)
    )
    $process = Start-Process -FilePath $InstallerPath -ArgumentList $arguments -WorkingDirectory $ToolsRoot -Wait -PassThru
    if ($process.ExitCode -ne 0) {
        throw "Inno Setup installer exited with code $($process.ExitCode); see $InstallLogPath"
    }

    Assert-OrdinaryDirectory $InstallRoot
    Assert-OrdinaryFile $CompilerPath
    $manifest = [ordered]@{
        version = $Version
        source = $DownloadUri
        installerSha256 = $InstallerSha256
        compilerSha256 = (Get-FileHash -LiteralPath $CompilerPath -Algorithm SHA256).Hash.ToLowerInvariant()
    }
    [IO.File]::WriteAllText($ManifestPath, (($manifest | ConvertTo-Json) + "`n"), (New-Object Text.UTF8Encoding($false)))
    Assert-Installation
    Write-Output "Installed Inno Setup $Version at $InstallRoot"
}

switch ($Action) {
    'install' {
        Install-InnoSetup
    }
    'status' {
        $null = Get-ManagedCompilerPath
        Write-Output "Inno Setup $Version is ready at $InstallRoot"
    }
    'path' {
        Write-Output (Get-ManagedCompilerPath)
    }
    'reset' {
        if (-not $Force) {
            throw 'reset deletes the managed Inno Setup installation; pass -Force to continue'
        }
        Remove-OwnedDirectory $InstallRoot
        Remove-Item -LiteralPath $InstallLogPath -Force -ErrorAction SilentlyContinue
        Write-Output "Removed managed Inno Setup installation at $InstallRoot; retained verified installer cache $InstallerPath"
    }
}
