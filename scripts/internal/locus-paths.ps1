function Resolve-LocusUserRoot {
    [CmdletBinding()]
    param(
        [string]$Override
    )

    if ([string]::IsNullOrWhiteSpace($Override)) {
        if ([string]::IsNullOrWhiteSpace($HOME)) {
            throw 'cannot resolve the user Locus directory because HOME is empty'
        }
        $candidate = Join-Path $HOME '.locus'
    }
    else {
        $candidate = $Override
    }

    $fullPath = [IO.Path]::GetFullPath($candidate)
    $trimmedPath = $fullPath.TrimEnd([char[]]@([IO.Path]::DirectorySeparatorChar, [IO.Path]::AltDirectorySeparatorChar))
    if ([IO.Path]::GetFileName($trimmedPath) -ne '.locus') {
        throw "user Locus root must end with '.locus': $fullPath"
    }
    if (Test-Path -LiteralPath $trimmedPath) {
        $item = Get-Item -LiteralPath $trimmedPath -Force
        if (-not $item.PSIsContainer) {
            throw "user Locus root is not a directory: $trimmedPath"
        }
        if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw "user Locus root must not be a symbolic link or junction: $trimmedPath"
        }
    }
    return $trimmedPath
}
