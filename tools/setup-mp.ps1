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
# Phase 2.10.2: gotuber-camera.exe からの自動呼び出しにも対応。
# -Force 以外は冪等 (既存 venv があればスキップ)。
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
    $modelPath = Join-Path $RootDir "assets\models\face_landmarker.task"

    if (-not (Test-Path $requirements)) {
        throw "$requirements が見つかりません。"
    }

    if (-not (Test-Path $modelPath)) {
        throw "同梱モデル $modelPath が見つかりません。リポジトリを取り直すか、docs/PHASE2.md Section 2.9 を確認してください。"
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

    # Phase 2.10.2: pip/wheel/setuptools を事前アップグレード。
    # 'Preparing metadata (pyproject.toml)' の CPU 99% を軽減するため
    # --prefer-binary を使用し、ビルド済み wheel を優先する。
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

    $model = Get-Item -LiteralPath $modelPath
    Write-Host "--- Bundled MediaPipe model ---" -ForegroundColor Yellow
    Write-Host ("{0} ({1} bytes)" -f $model.FullName, $model.Length)

    Write-Host ""
    Write-Host "Phase 2 MediaPipe 環境セットアップ完了。Activate: .venv-mp\Scripts\Activate.ps1" -ForegroundColor Green
}
finally {
    Pop-Location
}
