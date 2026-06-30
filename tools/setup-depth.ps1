# setup-depth.ps1 - GoTuber Phase 3.6 Depth Anything v3 Python 環境セットアップ (Windows)
#
# Usage:
#   .\tools\setup-depth.ps1
#   .\tools\setup-depth.ps1 -Force

[CmdletBinding()]
param(
    [switch]$Force
)

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RootDir = Split-Path -Parent $ScriptDir

Push-Location $RootDir
try {
    Write-Host "=== Phase 3.6 Depth Anything v3 environment setup (Windows) ===" -ForegroundColor Cyan

    $python = $null
    foreach ($candidate in @('python', 'py', 'python3')) {
        $cmd = Get-Command $candidate -ErrorAction SilentlyContinue
        if (-not $cmd) { continue }
        try {
            $versionOutput = & $cmd.Path --version 2>&1
        } catch {
            continue
        }
        if ($LASTEXITCODE -ne 0) { continue }
        if ($versionOutput -match 'Python (\d+)\.(\d+)') {
            $major = [int]$Matches[1]
            $minor = [int]$Matches[2]
            if ($major -ge 4 -or ($major -eq 3 -and $minor -ge 10)) {
                $python = $cmd.Path
                break
            }
        }
    }

    if (-not $python) {
        throw "Python 3.10+ が見つかりません。winget install Python.Python.3.12 でインストールし、PATH を通してください。"
    }

    $venvDir = Join-Path $RootDir ".venv-depth"
    $requirements = Join-Path $RootDir "tools\requirements-depth.txt"

    if (-not (Test-Path $requirements)) {
        throw "$requirements が見つかりません。"
    }

    if ($Force -and (Test-Path $venvDir)) {
        Write-Host "--- Removing existing venv ---" -ForegroundColor Yellow
        Remove-Item -Recurse -Force $venvDir
    }

    if (Test-Path $venvDir) {
        Write-Host ".venv-depth\ already exists — skipping. Use -Force to recreate." -ForegroundColor Green
        Write-Host "Activate: .venv-depth\Scripts\Activate.ps1"
        return
    }

    Write-Host "--- Creating venv: $venvDir (using $python) ---" -ForegroundColor Yellow
    & $python -m venv $venvDir
    if ($LASTEXITCODE -ne 0) {
        throw "venv creation failed"
    }

    $pip = Join-Path $venvDir "Scripts\pip.exe"

    Write-Host "--- Upgrading pip / wheel / setuptools ---" -ForegroundColor Yellow
    & $pip install --upgrade pip wheel setuptools
    if ($LASTEXITCODE -ne 0) {
        throw "pip upgrade failed"
    }

    Write-Host "--- Installing requirements from $requirements (prefer-binary) ---" -ForegroundColor Yellow
    & $pip install --prefer-binary -r $requirements
    if ($LASTEXITCODE -ne 0) {
        throw "pip install failed"
    }

    Write-Host ""
    Write-Host "Phase 3.6 Depth Anything v3 環境セットアップ完了。Activate: .venv-depth\Scripts\Activate.ps1" -ForegroundColor Green
}
finally {
    Pop-Location
}
