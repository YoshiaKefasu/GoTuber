# build.ps1 - GoTuber Windows native build (PowerShell)
#
# Usage:
#   .\scripts\build.ps1                    # リリースビルド
#   .\scripts\build.ps1 -Dev               # デバッグビルド (-ldflags なし)
#   .\scripts\build.ps1 -Clean             # ビルド前に bin/ 削除
#   .\scripts\build.ps1 -SkipTest          # テストスキップ
#   .\scripts\build.ps1 -Camera            # Phase 2 camera 有効ビルド (Windows native)
#
# Notes:
#   - 通常 build は Windows ネイティブ (mingw-w64 CGo)
#   - Camera build は Windows ネイティブ + `-tags camera`
#     → bin/gotuber-camera.exe が生成される
#   - Phase 2.10: CameraTracker (webcam) と ZeroMQ 依存を除去したため、
#     Windows native camera build が可能になった
#
# Requirements:
#   - Go 1.25+ (実要件は go.mod の go ディレクティブを参照。Phase 1.9 時点で 1.26 系)
#   - mingw-w64 (gcc: x86_64-w64-mingw32-gcc) for CGo (malgo)
#     Install via scoop: scoop install mingw
#   - Camera build: 追加の native 依存なし (Python sidecar 実行時は Python 3 + mediapipe/opencv)

[CmdletBinding()]
param(
    [switch]$Dev,
    [switch]$Clean,
    [switch]$SkipTest,
    [switch]$Camera
)

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RootDir = Split-Path -Parent $ScriptDir

# ── 通常 build / Camera build: Windows ネイティブ ──────────────────
# 環境変数を保存 (finally で復元)
$origGOOS = $env:GOOS
$origCGO = $env:CGO_ENABLED
$origCC = $env:CC

Push-Location $RootDir

try {
    if ($Camera) {
        Write-Host "=== GoTuber camera build (Windows native) ===" -ForegroundColor Cyan
        Write-Host "Phase 2.10: CameraTracker (webcam) 依存を除去済み" -ForegroundColor Green
    } else {
        Write-Host "=== GoTuber build (Windows) ===" -ForegroundColor Cyan
    }

    if ($Clean) {
        if (Test-Path "bin") {
            Write-Host "Cleaning bin/"
            Remove-Item -Recurse -Force "bin"
        }
    }

    if (-not (Test-Path "bin")) {
        New-Item -ItemType Directory -Path "bin" | Out-Null
    }

    if (-not $SkipTest) {
        Write-Host "--- go test ---" -ForegroundColor Yellow
        go test ./... 2>&1
        if ($LASTEXITCODE -ne 0) {
            throw "go test failed"
        }
    }

    Write-Host "--- go vet ---" -ForegroundColor Yellow
    go vet ./... 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw "go vet failed"
    }

    Write-Host "--- Windows build (mingw-w64 CGo) ---" -ForegroundColor Yellow
    $env:GOOS = "windows"
    $env:CGO_ENABLED = "1"
    $env:CC = "x86_64-w64-mingw32-gcc"

    if ($Camera) {
        $outFile = "bin/gotuber-camera.exe"
    } else {
        $outFile = "bin/gotuber.exe"
    }

    # Invoke-Expression を避け、配列 splatting で安全に渡す
    $goArgs = @('build')
    if (-not $Dev) {
        $goArgs += @('-ldflags', '-s -w')
    }
    if ($Camera) {
        $goArgs += @('-tags', 'camera')
    }
    $goArgs += @('-o', $outFile, './cmd/gotuber')
    & go @goArgs
    if ($LASTEXITCODE -ne 0) {
        if ($Camera) {
            throw "go build failed"
        }
        throw "go build failed"
    }

    $size = (Get-Item $outFile).Length
    Write-Host ""
    if ($Camera) {
        Write-Host "OK: $outFile ($([math]::Round($size / 1MB, 2)) MB) [Camera mode]" -ForegroundColor Green
    } else {
        Write-Host "OK: $outFile ($([math]::Round($size / 1MB, 2)) MB)" -ForegroundColor Green
    }
}
finally {
    Pop-Location
    # 環境変数を復元
    $env:GOOS = $origGOOS
    $env:CGO_ENABLED = $origCGO
    $env:CC = $origCC
}
