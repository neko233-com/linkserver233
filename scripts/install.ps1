param(
    [Parameter(Position = 0)]
    [string]$Version = "latest"
)

$ErrorActionPreference = 'Stop'

$BinaryName = 'linkserver233'
$Repo = 'neko233-com/linkserver233'
$InstallDir = Join-Path $env:LOCALAPPDATA $BinaryName

function Get-NormalizedVersion([string]$Value) {
    $normalized = $Value.Trim()
    while ($normalized.StartsWith('v') -or $normalized.StartsWith('V')) {
        $normalized = $normalized.Substring(1)
    }
    return $normalized
}

function Get-Architecture() {
    $arch = $env:PROCESSOR_ARCHITECTURE
    if ([string]::IsNullOrWhiteSpace($arch)) {
        return 'amd64'
    }

    switch ($arch.ToLowerInvariant()) {
        'amd64' { return 'amd64' }
        'x86' { return 'amd64' }
        'arm64' { return 'arm64' }
        default { return 'amd64' }
    }
}

function Test-PathInUserPath([string]$Dir) {
    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if ([string]::IsNullOrWhiteSpace($userPath)) {
        return $false
    }

    $normalizedDir = (Resolve-Path -LiteralPath $Dir).Path.TrimEnd('\')
    foreach ($entry in $userPath -split ';') {
        if ([string]::IsNullOrWhiteSpace($entry)) {
            continue
        }

        try {
            $normalizedEntry = (Resolve-Path -LiteralPath $entry -ErrorAction Stop).Path.TrimEnd('\')
            if ($normalizedEntry -ieq $normalizedDir) {
                return $true
            }
        } catch {
            if ($entry.TrimEnd('\') -ieq $normalizedDir) {
                return $true
            }
        }
    }

    return $false
}

function Add-ToUserPath([string]$Dir) {
    if (Test-PathInUserPath $Dir) {
        return $false
    }

    $current = [Environment]::GetEnvironmentVariable('Path', 'User')
    if ([string]::IsNullOrWhiteSpace($current)) {
        [Environment]::SetEnvironmentVariable('Path', $Dir, 'User')
    } else {
        [Environment]::SetEnvironmentVariable('Path', "$Dir;$current", 'User')
    }

    return $true
}

function Link-Binary([string]$Source, [string]$TargetDir) {
    $target = Join-Path $TargetDir "$BinaryName.exe"
    if (Test-Path -LiteralPath $target) {
        Remove-Item -LiteralPath $target -Force
    }

    try {
        New-Item -ItemType HardLink -Path $target -Target $Source -Force | Out-Null
    } catch {
        Copy-Item -LiteralPath $Source -Destination $target -Force
    }

    return $target
}

function Refresh-ProcessPath() {
    try {
        $machine = [Environment]::GetEnvironmentVariable('Path', 'Machine')
        $user = [Environment]::GetEnvironmentVariable('Path', 'User')
        $env:Path = "$machine;$user"
    } catch {
    }
}

$arch = Get-Architecture
$asset = "${BinaryName}-windows-$arch.exe"

if ([string]::IsNullOrWhiteSpace($Version) -or $Version -eq 'latest') {
    $url = "https://github.com/$Repo/releases/latest/download/$asset"
    $versionLabel = 'latest'
} else {
    $normalizedVersion = Get-NormalizedVersion $Version
    $url = "https://github.com/$Repo/releases/download/v$normalizedVersion/$asset"
    $versionLabel = "v$normalizedVersion"
}

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
$destination = Join-Path $InstallDir "$BinaryName.exe"

Write-Host "Installing $BinaryName $versionLabel for windows/$arch..."
Write-Host "Downloading $url..."
Invoke-WebRequest -Uri $url -OutFile $destination

$pathCandidates = @(
    (Join-Path $env:USERPROFILE '.local\bin'),
    (Join-Path $env:LOCALAPPDATA 'Microsoft\WinGet\Links'),
    (Join-Path $env:USERPROFILE 'go\bin')
)

$linked = $false
foreach ($candidate in $pathCandidates) {
    if (-not (Test-Path -LiteralPath $candidate)) {
        continue
    }
    if (-not (Test-PathInUserPath $candidate)) {
        continue
    }

    $linkPath = Link-Binary -Source $destination -TargetDir $candidate
    Write-Host "Linked $linkPath -> $destination"
    $linked = $true
    break
}

if (-not $linked) {
    if (Add-ToUserPath $InstallDir) {
        Write-Host "Added $InstallDir to the user PATH."
    } else {
        Write-Host "$InstallDir is already in the user PATH."
    }
}

Refresh-ProcessPath

Write-Host ''
Write-Host "Installed to $destination"
Write-Host 'Restart your terminal, then run: linkserver233 version'
