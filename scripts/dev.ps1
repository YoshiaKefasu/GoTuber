# dev.ps1 - GoTuber dev loop (Windows)
#
# Usage:
#   .\scripts\dev.ps1                 # デバッグビルド + 実行
#   .\scripts\dev.ps1 -NoRun           # ビルドのみ
#   .\scripts\dev.ps1 -Camera          # Phase 2 camera 有効ビルド + 実行 (Windows native)

[CmdletBinding()]
param(
    [switch]$NoRun,
    [switch]$Camera
)

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RootDir = Split-Path -Parent $ScriptDir

# build.ps1 内部で $ErrorActionPreference="Stop" のため、throw で例外が伝播する
# (別途 $LASTEXITCODE チェックは不要)
& "$ScriptDir\build.ps1" -Dev -SkipTest -Camera:$Camera

if ($NoRun) {
    Write-Host "OK: build only"
    exit 0
}

Write-Host ""
if ($Camera) {
    Write-Host "=== Running GoTuber (camera mode) ===" -ForegroundColor Cyan
    & "$RootDir\bin\gotuber-camera.exe"
} else {
    Write-Host "=== Running GoTuber ===" -ForegroundColor Cyan
    & "$RootDir\bin\gotuber.exe"
}
