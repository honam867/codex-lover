Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
$goCmd = Get-Command go -ErrorAction SilentlyContinue
if (-not $goCmd) {
    $fallbackGo = "C:\Program Files\Go\bin\go.exe"
    if (Test-Path $fallbackGo) {
        $goCmd = Get-Item $fallbackGo
    }
}

if (-not $goCmd) {
    throw "Go was not found. Install Go, then run .\install.cmd again."
}

$goExe = $null
if ($goCmd.PSObject.Properties.Name -contains "Source") {
    $goExe = $goCmd.Source
}
elseif ($goCmd.PSObject.Properties.Name -contains "FullName") {
    $goExe = $goCmd.FullName
}

if ([string]::IsNullOrWhiteSpace($goExe)) {
    throw "Could not resolve the Go executable path."
}

$binDir = Join-Path $env:LOCALAPPDATA "codex-lover\bin"
New-Item -ItemType Directory -Force -Path $binDir | Out-Null

$exePath = Join-Path $binDir "codex-lover.exe"
Push-Location $repoRoot
try {
    & $goExe build -o $exePath .\cmd\codex-lover
    if ($LASTEXITCODE -ne 0) {
        throw "go build failed with exit code $LASTEXITCODE"
    }
}
finally {
    Pop-Location
}

$shimPath = Join-Path $binDir "codex-lover.cmd"
$shimContent = "@echo off`r`n`"$exePath`" %*`r`n"
[System.IO.File]::WriteAllText($shimPath, $shimContent)

$userBinDir = Join-Path $env:USERPROFILE "bin"
$installedUserBinShim = $false
if (Test-Path $userBinDir) {
    $userBinShimPath = Join-Path $userBinDir "codex-lover.cmd"
    [System.IO.File]::WriteAllText($userBinShimPath, $shimContent)
    $installedUserBinShim = $true
}

$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ([string]::IsNullOrWhiteSpace($userPath)) {
    $userPath = ""
}

$parts = @()
foreach ($segment in ($userPath -split ';')) {
    if (-not [string]::IsNullOrWhiteSpace($segment)) {
        $parts += $segment.Trim()
    }
}

$alreadyPresent = $false
foreach ($segment in $parts) {
    if ($segment.TrimEnd('\').ToLowerInvariant() -eq $binDir.TrimEnd('\').ToLowerInvariant()) {
        $alreadyPresent = $true
        break
    }
}

if (-not $alreadyPresent) {
    $parts += $binDir
    $newPath = ($parts -join ';')
    [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
    Set-ItemProperty -Path "HKCU:\Environment" -Name "Path" -Value $newPath
}

Write-Host ""
Write-Host "Installed codex-lover to $exePath"
Write-Host "Shim created at $shimPath"
if ($installedUserBinShim) {
    Write-Host "Shim also created at $userBinDir\\codex-lover.cmd"
}
Write-Host ""
Write-Host "If this terminal was opened before install, run:"
Write-Host "  set PATH=$binDir;%PATH%"
Write-Host ""
Write-Host "Or just open a new terminal and run:"
Write-Host "  codex-lover"
