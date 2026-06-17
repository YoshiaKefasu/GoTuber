# setup-mp.ps1 - GoTuber Phase 2 MediaPipe Python 環境セットアップ (Windows)
#
# Usage:
#   .\tools\setup-mp.ps1                # .venv-mp\ を作成 + tools\requirements-mp.txt を pip install
#   .\tools\setup-mp.ps1 -Force         # 既存 .venv-mp\ を削除して再作成
#
# Requirements:
#   - Python 3.9+ (mediapipe 0.10.x 要件)
#     Install: winget install Python.Python.3.12
#
# 詳細: docs/PHASE2.md Section 4.4

[CmdletBinding()]
param(
    [switch]$Force
)

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RootDir = Split-Path -Parent $ScriptDir

Push-Location $RootDir
try {
    Write-Host "=== Phase 2 MediaPipe environment setup (Windows) ===" -ForegroundColor Cyan

    # Python 3.9+ 検出 (python → py → python3 の順)
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
            # forward-compat: Python 4.0+ も許可 (bash 版 setup-mp.sh:44 の sys.version_info >= (3, 9) と挙動を揃える)
            if ($major -ge 4 -or ($major -eq 3 -and $minor -ge 9)) {
                $python = $cmd.Path
                break
            }
        }
    }

    if (-not $python) {
        throw "Python 3.9+ が見つかりません。winget install Python.Python.3.12 でインストールし、PATH を通してください。"
    }

    $venvDir = Join-Path $RootDir ".venv-mp"
    $requirements = Join-Path $RootDir "tools\requirements-mp.txt"

    if (-not (Test-Path $requirements)) {
        throw "$requirements が見つかりません。"
    }

    if ($Force -and (Test-Path $venvDir)) {
        Write-Host "--- Removing existing venv ---" -ForegroundColor Yellow
        Remove-Item -Recurse -Force $venvDir
    }

    if (Test-Path $venvDir) {
        Write-Host ".venv-mp\ already exists — skipping. Use -Force to recreate." -ForegroundColor Green
        Write-Host "Activate: .venv-mp\Scripts\Activate.ps1"
        return
    }

    Write-Host "--- Creating venv: $venvDir (using $python) ---" -ForegroundColor Yellow
    & $python -m venv $venvDir
    if ($LASTEXITCODE -ne 0) {
        throw "venv creation failed"
    }

    $pip = Join-Path $venvDir "Scripts\pip.exe"

    Write-Host "--- Upgrading pip ---" -ForegroundColor Yellow
    & $pip install --upgrade pip
    if ($LASTEXITCODE -ne 0) {
        throw "pip upgrade failed"
    }

    Write-Host "--- Installing requirements from $requirements ---" -ForegroundColor Yellow
    & $pip install -r $requirements
    if ($LASTEXITCODE -ne 0) {
        throw "pip install failed"
    }

    Write-Host ""
    Write-Host "Phase 2 MediaPipe 環境セットアップ完了。Activate: .venv-mp\Scripts\Activate.ps1" -ForegroundColor Green
}
finally {
    Pop-Location
}
