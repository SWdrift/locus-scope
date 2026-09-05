[CmdletBinding()]
param(
    [string]$UninstallerPath,
    [switch]$Interactive
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$RepositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$TempRoot = Join-Path $RepositoryRoot 'temp'
New-Item -ItemType Directory -Force -Path $TempRoot | Out-Null
$tempItem = Get-Item -LiteralPath $TempRoot -Force
if (($tempItem.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
    throw "uninstaller log directory must not be a symbolic link or junction: $TempRoot"
}
if ([string]::IsNullOrWhiteSpace($UninstallerPath)) {
    if ([string]::IsNullOrWhiteSpace($HOME)) {
        throw 'cannot resolve the user Locus directory because HOME is empty'
    }
    $UninstallerPath = Join-Path $HOME '.locus\installer\unins000.exe'
}
$UninstallerPath = (Resolve-Path -LiteralPath $UninstallerPath).Path
if ([IO.Path]::GetExtension($UninstallerPath) -ne '.exe') {
    throw "uninstaller path must name an .exe file: $UninstallerPath"
}

$arguments = @(
    '/NORESTART',
    "/LOG=$(Join-Path $RepositoryRoot 'temp\uninstall-user.log')"
)
if (-not $Interactive) {
    $arguments += '/VERYSILENT', '/SUPPRESSMSGBOXES'
}

$process = Start-Process -FilePath $UninstallerPath -ArgumentList $arguments -Wait -PassThru
if ($process.ExitCode -ne 0) {
    throw "Locus uninstaller exited with code $($process.ExitCode); see temp/uninstall-user.log"
}

Write-Output 'Uninstalled Locus programs for the current user; user data was preserved'
