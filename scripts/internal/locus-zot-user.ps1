[CmdletBinding()]
param(
    [Parameter(Position = 0)]
    [ValidateSet('start', 'stop', 'status', 'enable-autostart', 'disable-autostart')]
    [string]$Action = 'status'
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$LocusRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
if ([IO.Path]::GetFileName($LocusRoot.TrimEnd([IO.Path]::DirectorySeparatorChar)) -ne '.locus') {
    throw "Zot manager must be installed under a .locus directory: $PSScriptRoot"
}

$StateRoot = Join-Path $LocusRoot 'zot'
$BinaryPath = Join-Path $StateRoot 'bin\zot.exe'
$ConfigPath = Join-Path $StateRoot 'config.json'
$LogRoot = Join-Path $StateRoot 'logs'
$RegistryRoot = Join-Path $StateRoot 'registry'
$PidPath = Join-Path $StateRoot 'zot.pid'
$ReadyUrl = 'http://127.0.0.1:18080/readyz'
$RegistryUrl = 'http://127.0.0.1:18080/v2/'
$ScheduledTaskName = 'Locus Zot'

function Assert-ZotBinary {
    if (-not (Test-Path -LiteralPath $BinaryPath -PathType Leaf)) {
        throw "Zot is not installed at $BinaryPath"
    }
}

function Initialize-ZotConfiguration {
    New-Item -ItemType Directory -Force -Path $LogRoot, $RegistryRoot | Out-Null
    if (Test-Path -LiteralPath $ConfigPath -PathType Leaf) {
        return
    }

    $configuration = [ordered]@{
        distSpecVersion = '1.1.1'
        storage = [ordered]@{
            rootDirectory = $RegistryRoot
        }
        http = [ordered]@{
            address = '127.0.0.1'
            port = '18080'
        }
        log = [ordered]@{
            level = 'info'
        }
    }
    $json = $configuration | ConvertTo-Json -Depth 4
    [IO.File]::WriteAllText($ConfigPath, $json, (New-Object Text.UTF8Encoding($false)))
}

function Get-ZotProcess {
    if (-not (Test-Path -LiteralPath $PidPath -PathType Leaf)) {
        return $null
    }

    $pidText = (Get-Content -LiteralPath $PidPath -Raw).Trim()
    $processId = 0
    if (-not [int]::TryParse($pidText, [ref]$processId) -or $processId -le 0) {
        throw "Invalid Zot PID file: $PidPath"
    }

    $process = Get-Process -Id $processId -ErrorAction SilentlyContinue
    if ($null -eq $process) {
        Remove-Item -LiteralPath $PidPath -Force
        return $null
    }

    $actualPath = $process.Path
    if ([string]::IsNullOrWhiteSpace($actualPath)) {
        throw "Cannot determine the executable for Zot PID $processId"
    }
    $expectedPath = (Resolve-Path -LiteralPath $BinaryPath).Path
    $resolvedActualPath = (Resolve-Path -LiteralPath $actualPath).Path
    if (-not [string]::Equals($resolvedActualPath, $expectedPath, [StringComparison]::OrdinalIgnoreCase)) {
        throw "PID $processId belongs to '$resolvedActualPath', not '$expectedPath'"
    }

    return $process
}

function Test-ZotEndpoint {
    param([Parameter(Mandatory)][string]$Uri)

    try {
        $response = Invoke-WebRequest -UseBasicParsing -Uri $Uri -Method Get -TimeoutSec 2
        return $response.StatusCode -eq 200
    }
    catch {
        return $false
    }
}

function Test-ZotReady {
    return (Test-ZotEndpoint $ReadyUrl) -and (Test-ZotEndpoint $RegistryUrl)
}

function Assert-ZotConfiguration {
    Initialize-ZotConfiguration
    Push-Location $StateRoot
    try {
        & $BinaryPath verify $ConfigPath
        if ($LASTEXITCODE -ne 0) {
            throw "zot verify exited with code $LASTEXITCODE"
        }
    }
    finally {
        Pop-Location
    }
}

function Start-Zot {
    Assert-ZotBinary
    $existing = Get-ZotProcess
    if ($null -ne $existing) {
        if (Test-ZotReady) {
            Write-Output "Zot is already running with PID $($existing.Id)"
            return
        }
        throw "Zot PID $($existing.Id) is running but the registry is not ready"
    }

    Assert-ZotConfiguration
    $timestamp = Get-Date -Format 'yyyyMMdd-HHmmss'
    $stdoutPath = Join-Path $LogRoot "zot-$timestamp.stdout.log"
    $stderrPath = Join-Path $LogRoot "zot-$timestamp.stderr.log"
    $process = Start-Process -FilePath $BinaryPath -ArgumentList @('serve', ('"{0}"' -f $ConfigPath)) -WorkingDirectory $StateRoot -RedirectStandardOutput $stdoutPath -RedirectStandardError $stderrPath -WindowStyle Hidden -PassThru
    [IO.File]::WriteAllText($PidPath, [string]$process.Id)

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

function Get-ZotStatus {
    Assert-ZotBinary
    $process = Get-ZotProcess
    if ($null -eq $process) {
        Write-Output 'Zot is not running'
        return
    }
    if (-not (Test-ZotReady)) {
        throw "Zot PID $($process.Id) is running but /readyz or /v2/ is unavailable"
    }
    Write-Output "Zot is ready with PID $($process.Id)"
}

function Enable-ZotAutoStart {
    Assert-ZotBinary
    $hostPath = (Get-Process -Id $PID).Path
    $arguments = '-NoLogo -NoProfile -NonInteractive -WindowStyle Hidden -ExecutionPolicy Bypass -File "{0}" start' -f $PSCommandPath
    $taskAction = New-ScheduledTaskAction -Execute $hostPath -Argument $arguments
    $taskTrigger = New-ScheduledTaskTrigger -AtLogOn
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent().Name
    $principal = New-ScheduledTaskPrincipal -UserId $identity -LogonType Interactive -RunLevel Limited
    $task = New-ScheduledTask -Action $taskAction -Trigger $taskTrigger -Principal $principal
    Register-ScheduledTask -TaskName $ScheduledTaskName -InputObject $task -Force | Out-Null
    Write-Output 'Zot login startup is enabled'
}

function Disable-ZotAutoStart {
    Unregister-ScheduledTask -TaskName $ScheduledTaskName -Confirm:$false -ErrorAction SilentlyContinue
    Write-Output 'Zot login startup is disabled'
}

switch ($Action) {
    'start' { Start-Zot }
    'stop' { Stop-Zot }
    'status' { Get-ZotStatus }
    'enable-autostart' { Enable-ZotAutoStart }
    'disable-autostart' { Disable-ZotAutoStart }
}
