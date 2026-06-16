# dev.ps1 - GoTuber dev loop (Windows)
#
# Usage:
#   .\scripts\dev.ps1                 # デバッグビルド + 実行
#   .\scripts\dev.ps1 -NoRun           # ビルドのみ

[CmdletBinding()]
param(
    [switch]$NoRun
)

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RootDir = Split-Path -Parent $ScriptDir

# build.ps1 内部で $ErrorActionPreference="Stop" のため、throw で例外が伝播する
# (別途 $LASTEXITCODE チェックは不要)
& "$ScriptDir\build.ps1" -Dev -SkipTest

if ($NoRun) {
    Write-Host "OK: build only"
    exit 0
}

Write-Host ""
Write-Host "=== Running GoTuber ===" -ForegroundColor Cyan
& "$RootDir\bin\gotuber.exe"
