[CmdletBinding()]
param(
    [Parameter(Position = 0)]
    [ValidateSet('install', 'verify', 'serve', 'start', 'stop', 'status')]
    [string]$Action = 'status'
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$ZotVersion = 'v2.1.20'
$ZotAsset = 'zot-windows-amd64-minimal.exe'
$ZotSha256 = '80d42edb8c2b65054f43a113da7d00c78a8491d974b8edd3680a316d471f085c'
$ReleaseBase = "https://github.com/project-zot/zot/releases/download/$ZotVersion"
$RepositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$ConfigPath = Join-Path $RepositoryRoot 'deploy\local-zot\config.json'
$StateRoot = Join-Path $RepositoryRoot 'temp\zot'
$DownloadRoot = Join-Path $StateRoot 'download'
$BinaryRoot = Join-Path $StateRoot 'bin'
$LogRoot = Join-Path $StateRoot 'logs'
$BinaryPath = Join-Path $BinaryRoot 'zot.exe'
$ChecksumPath = Join-Path $DownloadRoot 'checksums.sha256.txt'
$DownloadPath = Join-Path $DownloadRoot $ZotAsset
$PidPath = Join-Path $StateRoot 'zot.pid'
$ReadyUrl = 'http://127.0.0.1:18080/readyz'
$RegistryUrl = 'http://127.0.0.1:18080/v2/'

function New-ZotDirectories {
    New-Item -ItemType Directory -Force -Path $DownloadRoot, $BinaryRoot, $LogRoot | Out-Null
}

function Get-ZotProcess {
    if (-not (Test-Path -LiteralPath $PidPath -PathType Leaf)) {
        return $null
    }

    $pidText = (Get-Content -LiteralPath $PidPath -Raw).Trim()
    $processId = 0
    if (-not [int]::TryParse($pidText, [ref]$processId) -or $processId -le 0) {
        throw "invalid Zot PID file: $PidPath"
    }

    $process = Get-Process -Id $processId -ErrorAction SilentlyContinue
    if ($null -eq $process) {
        Remove-Item -LiteralPath $PidPath -Force
        return $null
    }

    $actualPath = $process.Path
    if ([string]::IsNullOrWhiteSpace($actualPath) -or
        -not [string]::Equals((Resolve-Path -LiteralPath $actualPath).Path, (Resolve-Path -LiteralPath $BinaryPath).Path, [StringComparison]::OrdinalIgnoreCase)) {
        throw "PID $processId belongs to '$actualPath', not '$BinaryPath'"
    }

    return $process
}

function Test-ZotEndpoint {
    param([Parameter(Mandatory)][string]$Uri)

    try {
        $response = Invoke-WebRequest -Uri $Uri -Method Get -TimeoutSec 2
        return $response.StatusCode -eq 200
    }
    catch {
        return $false
    }
}

function Test-ZotReady {
    return (Test-ZotEndpoint $ReadyUrl) -and (Test-ZotEndpoint $RegistryUrl)
}

function Assert-ZotBinary {
    if (-not (Test-Path -LiteralPath $BinaryPath -PathType Leaf)) {
        throw "Zot is not installed; run: pwsh -File deploy/local-zot/zot.ps1 install"
    }

    $actualHash = (Get-FileHash -LiteralPath $BinaryPath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actualHash -ne $ZotSha256) {
        throw "installed Zot binary hash mismatch: expected $ZotSha256, got $actualHash"
    }
}

function Install-Zot {
    New-ZotDirectories

    if (Test-Path -LiteralPath $BinaryPath -PathType Leaf) {
        $existingHash = (Get-FileHash -LiteralPath $BinaryPath -Algorithm SHA256).Hash.ToLowerInvariant()
        if ($existingHash -eq $ZotSha256) {
            Write-Output "Zot $ZotVersion is already installed at $BinaryPath"
            return
        }
        throw "refusing to replace Zot binary with unexpected hash $existingHash at $BinaryPath"
    }

    Invoke-WebRequest -Uri "$ReleaseBase/checksums.sha256.txt" -OutFile $ChecksumPath
    Invoke-WebRequest -Uri "$ReleaseBase/$ZotAsset" -OutFile $DownloadPath

    $matchingLines = @(Get-Content -LiteralPath $ChecksumPath | Where-Object {
        $_ -match "^([0-9A-Fa-f]{64})\s+\*?$([regex]::Escape($ZotAsset))$"
    })
    if ($matchingLines.Count -ne 1) {
        throw "checksum manifest must contain exactly one entry for $ZotAsset"
    }

    $manifestHash = ([regex]::Match($matchingLines[0], '^([0-9A-Fa-f]{64})')).Groups[1].Value.ToLowerInvariant()
    if ($manifestHash -ne $ZotSha256) {
        throw "release checksum mismatch for ${ZotAsset}: expected $ZotSha256, got $manifestHash"
    }

    $downloadHash = (Get-FileHash -LiteralPath $DownloadPath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($downloadHash -ne $ZotSha256) {
        throw "downloaded Zot binary hash mismatch: expected $ZotSha256, got $downloadHash"
    }

    Move-Item -LiteralPath $DownloadPath -Destination $BinaryPath
    Write-Output "Installed Zot $ZotVersion at $BinaryPath"
}

function Verify-Zot {
    Assert-ZotBinary

    Push-Location $RepositoryRoot
    try {
        & $BinaryPath verify $ConfigPath
        if ($LASTEXITCODE -ne 0) {
            throw "zot verify exited with code $LASTEXITCODE"
        }

        $versionOutput = (& $BinaryPath --version 2>&1 | Out-String).Trim()
        if ($LASTEXITCODE -ne 0) {
            throw "zot --version exited with code $LASTEXITCODE"
        }
        if ($versionOutput -notmatch '(?<![0-9])v?2\.1\.20(?![0-9])') {
            throw "unexpected Zot version output: $versionOutput"
        }
        Write-Output $versionOutput
    }
    finally {
        Pop-Location
    }
}

function Serve-Zot {
    Verify-Zot
    Push-Location $RepositoryRoot
    try {
        & $BinaryPath serve $ConfigPath
        if ($LASTEXITCODE -ne 0) {
            throw "zot serve exited with code $LASTEXITCODE"
        }
    }
    finally {
        Pop-Location
    }
}

function Start-Zot {
    Assert-ZotBinary
    New-ZotDirectories

    $existing = Get-ZotProcess
    if ($null -ne $existing) {
        if (Test-ZotReady) {
            Write-Output "Zot is already running with PID $($existing.Id)"
            return
        }
        throw "Zot PID $($existing.Id) is running but the registry is not ready"
    }

    Verify-Zot
    $timestamp = Get-Date -Format 'yyyyMMdd-HHmmss'
    $stdoutPath = Join-Path $LogRoot "zot-$timestamp.stdout.log"
    $stderrPath = Join-Path $LogRoot "zot-$timestamp.stderr.log"
    $process = Start-Process -FilePath $BinaryPath -ArgumentList @('serve', ('"{0}"' -f $ConfigPath)) -WorkingDirectory $RepositoryRoot -RedirectStandardOutput $stdoutPath -RedirectStandardError $stderrPath -WindowStyle Hidden -PassThru
    Set-Content -LiteralPath $PidPath -Value $process.Id -NoNewline

    $deadline = [DateTime]::UtcNow.AddSeconds(15)
    do {
        if ($process.HasExited) {
            Remove-Item -LiteralPath $PidPath -Force -ErrorAction SilentlyContinue
            throw "Zot exited before becoming ready; see $stdoutPath and $stderrPath"
        }
        if (Test-ZotReady) {
            Write-Output "Zot is ready with PID $($process.Id)"
            return
        }
        Start-Sleep -Milliseconds 250
        $process.Refresh()
    } while ([DateTime]::UtcNow -lt $deadline)

    Stop-Process -Id $process.Id -Force -ErrorAction SilentlyContinue
    Wait-Process -Id $process.Id -Timeout 5 -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $PidPath -Force -ErrorAction SilentlyContinue
    throw "Zot did not become ready within 15 seconds; see $stdoutPath and $stderrPath"
}

function Get-ZotStatus {
    $process = Get-ZotProcess
    if ($null -eq $process) {
        throw 'Zot is not running'
    }
    if (-not (Test-ZotReady)) {
        throw "Zot PID $($process.Id) is running but /readyz or /v2/ is unavailable"
    }
    Write-Output "Zot is ready with PID $($process.Id)"
}

function Stop-Zot {
    $process = Get-ZotProcess
    if ($null -eq $process) {
        Write-Output 'Zot is already stopped'
        return
    }

    Stop-Process -Id $process.Id
    Wait-Process -Id $process.Id -Timeout 15 -ErrorAction SilentlyContinue
    if (-not $process.HasExited) {
        throw "Zot PID $($process.Id) did not stop within 15 seconds"
    }
    Remove-Item -LiteralPath $PidPath -Force -ErrorAction SilentlyContinue
    Write-Output 'Zot stopped'
}

switch ($Action) {
    'install' { Install-Zot }
    'verify' { Verify-Zot }
    'serve' { Serve-Zot }
    'start' { Start-Zot }
    'stop' { Stop-Zot }
    'status' { Get-ZotStatus }
}
