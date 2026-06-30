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

    # uv があれば、まず 3.12 → 3.11 → 3.10 → 3.13 を優先する。
    $uvCmd = Get-Command uv -ErrorAction SilentlyContinue
    if ($uvCmd) {
        foreach ($versionArg in @('3.12', '3.11', '3.10', '3.13')) {
            try {
                $uvPython = & $uvCmd.Path python find $versionArg 2>$null
                if ($LASTEXITCODE -eq 0 -and $uvPython -and (Test-Path $uvPython)) {
                    $python = $uvPython.Trim()
                    break
                }
            } catch {
                continue
            }
        }
    }

    # py launcher があれば、次に 3.13 → 3.12 → 3.11 → 3.10 を優先する。
    $pyLauncher = Get-Command py -ErrorAction SilentlyContinue
    if (-not $python -and $pyLauncher) {
        foreach ($versionArg in @('-3.12', '-3.11', '-3.10', '-3.13')) {
            try {
                & $pyLauncher.Path $versionArg --version *> $null
                if ($LASTEXITCODE -eq 0) {
                    $python = "$($pyLauncher.Path) $versionArg"
                    break
                }
            } catch {
                continue
            }
        }
    }

    foreach ($candidate in @('python', 'python3')) {
        if ($python) { break }
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
            if ($major -eq 3 -and $minor -ge 10 -and $minor -le 13) {
                $python = $cmd.Path
                break
            }
        }
    }

    if (-not $python) {
        throw "Python 3.10〜3.13 が見つかりません。py -3.12 か winget install Python.Python.3.12 を用意してください。"
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
    if ($python -is [string] -and $python.Contains(' -3.')) {
        $parts = $python.Split(' ', 2)
        & $parts[0] $parts[1] -m venv $venvDir
    } else {
        & $python -m venv $venvDir
    }
    if ($LASTEXITCODE -ne 0) {
        throw "venv creation failed"
    }

    $pythonVenv = Join-Path $venvDir "Scripts\python.exe"
    $pip = Join-Path $venvDir "Scripts\pip.exe"

    Write-Host "--- Upgrading pip / wheel / setuptools ---" -ForegroundColor Yellow
    & $pythonVenv -m pip install --upgrade pip wheel setuptools
    if ($LASTEXITCODE -ne 0) {
        throw "pip upgrade failed"
    }

    Write-Host "--- Installing CUDA-enabled PyTorch (cu121) ---" -ForegroundColor Yellow
    & $pip install torch torchvision --index-url https://download.pytorch.org/whl/cu121
    if ($LASTEXITCODE -ne 0) {
        throw "CUDA torch install failed"
    }

    Write-Host "--- Installing xformers (DA3 required dependency) ---" -ForegroundColor Yellow
    # xformers は torch バージョンに厳密に一致する必要がある。
    # torch 2.5.x → xformers 0.0.28.post3 (from source build)
    & $pip install "xformers==0.0.28.post3"
    if ($LASTEXITCODE -ne 0) {
        Write-Host "WARN: xformers install failed (DA3 may still work without memory-efficient attention)" -ForegroundColor Yellow
    }

    Write-Host "--- Installing remaining requirements from $requirements ---" -ForegroundColor Yellow
    & $pip install -r $requirements
    if ($LASTEXITCODE -ne 0) {
        throw "pip install failed"
    }

    # CUDA 利用可能か診断
    Write-Host ""
    Write-Host "--- Checking CUDA availability ---" -ForegroundColor Yellow
    $cudaCheck = & $pythonVenv -c @"
import sys
try:
    import torch
    cuda_ok = torch.cuda.is_available()
    if cuda_ok:
        name = torch.cuda.get_device_name(0)
        ver = torch.version.cuda
        print(f'OK: CUDA {ver} — {name}')
    else:
        print('WARN: torch.cuda.is_available() = False. CPU inference will be used.')
        print('      If you have an NVIDIA GPU, check that CUDA toolkit matches torch CUDA version.')
        print('      Manual fix: .venv-depth\\Scripts\\pip install torch --index-url https://download.pytorch.org/whl/cu121')
except ImportError:
    print('WARN: torch not installed — depth generation will not work.')
except Exception as e:
    print(f'WARN: CUDA check failed: {e}')
"@ 2>&1
    foreach ($line in $cudaCheck) {
        if ($line -match '^OK:') {
            Write-Host "  $line" -ForegroundColor Green
        } elseif ($line -match '^WARN:') {
            Write-Host "  $line" -ForegroundColor Yellow
        } else {
            Write-Host "  $line"
        }
    }

    Write-Host ""
    Write-Host "Phase 3.6 Depth Anything v3 環境セットアップ完了。Activate: .venv-depth\Scripts\Activate.ps1" -ForegroundColor Green
}
finally {
    Pop-Location
}
