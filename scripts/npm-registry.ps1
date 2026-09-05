[CmdletBinding()]
param(
    [Parameter(Position = 0)]
    [ValidateSet('install', 'start', 'stop', 'status', 'logs', 'reset')]
    [string]$Action = 'status',
    [switch]$Follow,
    [switch]$Force
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$RepositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$StateRoot = Join-Path $RepositoryRoot 'temp\verdaccio-dev'
$TempRoot = Join-Path $RepositoryRoot 'temp'
$StorageRoot = Join-Path $StateRoot 'storage'
$AuthRoot = Join-Path $StateRoot 'auth'
$LogRoot = Join-Path $StateRoot 'logs'
$ConfigPath = Join-Path $StateRoot 'config.yaml'
$ProcessRecordPath = Join-Path $StateRoot 'process.json'
$AuthPath = Join-Path $AuthRoot 'htpasswd'
$ServerLogPath = Join-Path $LogRoot 'verdaccio.log'
$StdoutLogPath = Join-Path $LogRoot 'stdout.log'
$StderrLogPath = Join-Path $LogRoot 'stderr.log'
$TemplatePath = Join-Path $PSScriptRoot 'internal\verdaccio.dev.yaml'
$VerdaccioEntryPath = Join-Path $RepositoryRoot 'node_modules\verdaccio\bin\verdaccio'
$VerdaccioManifestPath = Join-Path $RepositoryRoot 'node_modules\verdaccio\package.json'
$RegistryUrl = 'http://127.0.0.1:4873/'
$ReadyUrl = $RegistryUrl + '-/ping'

function Assert-RequiredFile {
    param([Parameter(Mandatory)][string]$Path)

    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        throw "required file is missing: $Path"
    }
}
function Assert-VerdaccioInstallation {
    Assert-RequiredFile $VerdaccioEntryPath
    Assert-RequiredFile $VerdaccioManifestPath
    $manifest = Get-Content -LiteralPath $VerdaccioManifestPath -Raw | ConvertFrom-Json
    if ($manifest.name -ne 'verdaccio' -or $manifest.version -ne '6.10.2') {
        throw "expected project-local verdaccio@6.10.2 at $VerdaccioManifestPath"
    }
}

function Assert-OrdinaryDirectory {
    param([Parameter(Mandatory)][string]$Path)

    if (-not (Test-Path -LiteralPath $Path)) {
        return
    }
    $item = Get-Item -LiteralPath $Path -Force
    if (-not $item.PSIsContainer -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw "registry state path must be an ordinary directory: $Path"
    }
}


function ConvertTo-YamlString {
    param([Parameter(Mandatory)][string]$Value)

    return ConvertTo-Json ([IO.Path]::GetFullPath($Value)) -Compress
}

function Initialize-RegistryState {
    Assert-RequiredFile $TemplatePath
    Assert-OrdinaryDirectory $TempRoot
    Assert-OrdinaryDirectory $StateRoot
    New-Item -ItemType Directory -Force -Path $TempRoot, $StateRoot, $StorageRoot, $AuthRoot, $LogRoot | Out-Null
    foreach ($path in $TempRoot, $StateRoot, $StorageRoot, $AuthRoot, $LogRoot) {
        Assert-OrdinaryDirectory $path
    }
    foreach ($path in $ConfigPath, $ProcessRecordPath, $AuthPath, $ServerLogPath, $StdoutLogPath, $StderrLogPath) {
        if (-not (Test-Path -LiteralPath $path)) {
            continue
        }
        $item = Get-Item -LiteralPath $path -Force
        if ($item.PSIsContainer -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw "registry state file must be an ordinary file: $path"
        }
    }

    $configuration = Get-Content -LiteralPath $TemplatePath -Raw
    $configuration = $configuration.Replace('__STORAGE_PATH__', (ConvertTo-YamlString $StorageRoot))
    $configuration = $configuration.Replace('__AUTH_PATH__', (ConvertTo-YamlString $AuthPath))
    $configuration = $configuration.Replace('__LOG_PATH__', (ConvertTo-YamlString $ServerLogPath))
    [IO.File]::WriteAllText($ConfigPath, $configuration, (New-Object Text.UTF8Encoding($false)))
}

function Test-RegistryReady {
    try {
        $response = Invoke-WebRequest -UseBasicParsing -Uri $ReadyUrl -Method Get -TimeoutSec 2
        return $response.StatusCode -eq 200
    }
    catch {
        return $false
    }
}

function Get-ManagedRegistryProcess {
    if (-not (Test-Path -LiteralPath $ProcessRecordPath -PathType Leaf)) {
        return $null
    }

    try {
        $record = Get-Content -LiteralPath $ProcessRecordPath -Raw | ConvertFrom-Json
    }
    catch {
        throw "invalid Verdaccio process record: $ProcessRecordPath"
    }
    if ($null -eq $record.pid -or $null -eq $record.executable -or $null -eq $record.entrypoint -or $null -eq $record.config) {
        throw "incomplete Verdaccio process record: $ProcessRecordPath"
    }

    $processId = 0
    if (-not [int]::TryParse([string]$record.pid, [ref]$processId) -or $processId -le 0) {
        throw "invalid Verdaccio PID in $ProcessRecordPath"
    }
    $process = Get-Process -Id $processId -ErrorAction SilentlyContinue
    if ($null -eq $process) {
        Remove-Item -LiteralPath $ProcessRecordPath -Force
        return $null
    }

    if ($IsWindows) {
        $processInfo = Get-CimInstance Win32_Process -Filter "ProcessId = $processId"
        if ($null -eq $processInfo) {
            throw "cannot inspect Verdaccio PID $processId"
        }
        $actualExecutable = [string]$processInfo.ExecutablePath
        $commandLine = [string]$processInfo.CommandLine
    }
    else {
        $actualExecutable = $process.Path
        if ($IsLinux -and (Test-Path -LiteralPath "/proc/$processId/cmdline" -PathType Leaf)) {
            $commandLine = [Text.Encoding]::UTF8.GetString([IO.File]::ReadAllBytes("/proc/$processId/cmdline")).Replace([char]0, ' ')
        }
        else {
            $psCommand = Get-Command ps -CommandType Application -ErrorAction Stop
            $commandLine = (& $psCommand.Source -p $processId -o 'command=' | Out-String).Trim()
            if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($commandLine)) {
                throw "cannot inspect Verdaccio PID $processId"
            }
        }
    }

    $recordedExecutable = [IO.Path]::GetFullPath([string]$record.executable)
    $actualExecutable = [IO.Path]::GetFullPath($actualExecutable)
    if (-not [string]::Equals($recordedExecutable, $actualExecutable, [StringComparison]::OrdinalIgnoreCase)) {
        throw "PID $processId executable '$actualExecutable' does not match '$recordedExecutable'"
    }
    if (-not [string]::Equals([IO.Path]::GetFullPath([string]$record.entrypoint), [IO.Path]::GetFullPath($VerdaccioEntryPath), [StringComparison]::OrdinalIgnoreCase) -or
        -not [string]::Equals([IO.Path]::GetFullPath([string]$record.config), [IO.Path]::GetFullPath($ConfigPath), [StringComparison]::OrdinalIgnoreCase)) {
        throw "Verdaccio process record does not belong to this workspace: $ProcessRecordPath"
    }

    $commandLine = [string]$commandLine
    foreach ($requiredArgument in $VerdaccioEntryPath, $ConfigPath, '127.0.0.1:4873') {
        if ($commandLine.IndexOf($requiredArgument, [StringComparison]::OrdinalIgnoreCase) -lt 0) {
            throw "PID $processId command line does not match this Verdaccio instance"
        }
    }
    return $process
}


function Install-Registry {
    $runningProcess = Get-ManagedRegistryProcess
    if ($null -ne $runningProcess) {
        throw "stop Verdaccio PID $($runningProcess.Id) before installing workspace dependencies"
    }
    Push-Location $RepositoryRoot
    try {
        & pnpm install --frozen-lockfile
        if ($LASTEXITCODE -ne 0) {
            throw "pnpm install --frozen-lockfile exited with code $LASTEXITCODE"
        }
    }
    finally {
        Pop-Location
    }

    Assert-VerdaccioInstallation
    Initialize-RegistryState
    Write-Output "Installed project dependencies and initialized Verdaccio state at $StateRoot"
}

function Start-Registry {
    Assert-VerdaccioInstallation
    Initialize-RegistryState

    $existing = Get-ManagedRegistryProcess
    if ($null -ne $existing) {
        if (Test-RegistryReady) {
            Write-Output "Verdaccio is ready at $RegistryUrl with PID $($existing.Id)"
            return
        }
        throw "Verdaccio PID $($existing.Id) is running but $ReadyUrl is unavailable"
    }
    if (Test-RegistryReady) {
        throw "$RegistryUrl is already served by a process not owned by this workspace"
    }

    $nodeCommand = Get-Command node -CommandType Application -ErrorAction Stop
    $nodePath = (Resolve-Path -LiteralPath $nodeCommand.Source).Path
    Remove-Item -LiteralPath $StdoutLogPath, $StderrLogPath -Force -ErrorAction SilentlyContinue
    $arguments = @(
        ('"{0}"' -f $VerdaccioEntryPath),
        '--config',
        ('"{0}"' -f $ConfigPath),
        '--listen',
        '127.0.0.1:4873'
    )
    $startParameters = @{
        FilePath = $nodePath
        ArgumentList = $arguments
        WorkingDirectory = $StateRoot
        RedirectStandardOutput = $StdoutLogPath
        RedirectStandardError = $StderrLogPath
        PassThru = $true
    }
    if ($IsWindows) {
        $startParameters.WindowStyle = 'Hidden'
    }
    $process = Start-Process @startParameters
    $record = [ordered]@{
        pid = $process.Id
        executable = [IO.Path]::GetFullPath($process.Path)
        entrypoint = $VerdaccioEntryPath
        config = $ConfigPath
        registry = $RegistryUrl
    }
    $record | ConvertTo-Json | Set-Content -LiteralPath $ProcessRecordPath -Encoding utf8NoBOM

    $deadline = [DateTime]::UtcNow.AddSeconds(30)
    do {
        $process.Refresh()
        if ($process.HasExited) {
            Remove-Item -LiteralPath $ProcessRecordPath -Force -ErrorAction SilentlyContinue
            throw "Verdaccio exited before becoming ready; see $StdoutLogPath and $StderrLogPath"
        }
        if (Test-RegistryReady) {
            Write-Output "Verdaccio is ready at $RegistryUrl with PID $($process.Id)"
            return
        }
        Start-Sleep -Milliseconds 250
    } while ([DateTime]::UtcNow -lt $deadline)

    Stop-Process -Id $process.Id -Force -ErrorAction SilentlyContinue
    Wait-Process -Id $process.Id -Timeout 5 -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $ProcessRecordPath -Force -ErrorAction SilentlyContinue
    throw "Verdaccio did not become ready within 30 seconds; see $StdoutLogPath and $StderrLogPath"
}

function Get-RegistryStatus {
    $process = Get-ManagedRegistryProcess
    if ($null -eq $process) {
        throw 'Verdaccio is not running'
    }
    if (-not (Test-RegistryReady)) {
        throw "Verdaccio PID $($process.Id) is running but $ReadyUrl is unavailable"
    }
    Write-Output "Verdaccio is ready at $RegistryUrl with PID $($process.Id)"
}

function Stop-Registry {
    $process = Get-ManagedRegistryProcess
    if ($null -eq $process) {
        Write-Output 'Verdaccio is already stopped'
        return
    }

    Stop-Process -Id $process.Id
    Wait-Process -Id $process.Id -Timeout 15 -ErrorAction SilentlyContinue
    $process.Refresh()
    if (-not $process.HasExited) {
        throw "Verdaccio PID $($process.Id) did not stop within 15 seconds"
    }
    Remove-Item -LiteralPath $ProcessRecordPath -Force -ErrorAction SilentlyContinue
    Write-Output 'Verdaccio stopped'
}

function Show-RegistryLogs {
    $paths = @($ServerLogPath, $StdoutLogPath, $StderrLogPath) |
        Where-Object { Test-Path -LiteralPath $_ -PathType Leaf }
    if ($paths.Count -eq 0) {
        Write-Output "No Verdaccio logs exist under $LogRoot"
        return
    }
    Get-Content -LiteralPath $paths -Wait:$Follow
}

function Reset-Registry {
    if (-not $Force) {
        throw 'reset deletes the project-local Registry state; pass -Force to continue'
    }
    foreach ($path in $TempRoot, $StateRoot) {
        if (-not (Test-Path -LiteralPath $path)) {
            continue
        }
        $item = Get-Item -LiteralPath $path -Force
        if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw "refusing to reset symbolic link or junction: $path"
        }
    }
    if (Test-Path -LiteralPath $StateRoot -PathType Container) {
        $reparsePoint = Get-ChildItem -LiteralPath $StateRoot -Force -Recurse |
            Where-Object { ($_.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0 } |
            Select-Object -First 1
        if ($null -ne $reparsePoint) {
            throw "refusing to reset state containing a symbolic link or junction: $($reparsePoint.FullName)"
        }
    }
    Stop-Registry
    if (Test-Path -LiteralPath $StateRoot) {
        Remove-Item -LiteralPath $StateRoot -Recurse -Force
    }
    Write-Output "Removed Verdaccio state at $StateRoot"
}

switch ($Action) {
    'install' { Install-Registry }
    'start' { Start-Registry }
    'stop' { Stop-Registry }
    'status' { Get-RegistryStatus }
    'logs' { Show-RegistryLogs }
    'reset' { Reset-Registry }
}
