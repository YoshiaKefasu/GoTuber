// Package killswitch provides graceful shutdown via OS signals.
//
// 終了トリガ:
//  1. SIGINT (Ctrl+C) — Unix 系での graceful 終了
//  2. SIGTERM        — Unix 系の systemd / docker 等から
//
// Windows は signal.Notify を登録しない (no-op):
//   - 理由: Phase 1.13 visual test で F1 / Esc 押下時にアプリが即終了する
//     バグが発覚。Ebitengine v2.9.9 のソース確認では SIGINT ハンドラは
//     存在せず、真因は未確定だが signal.Notify(os.Interrupt) との相互作用が
//     強く疑わしい。暫定措置として Windows では signal.Notify を完全に
//     スキップする。
//   - 終了方法:
//   - ウィンドウの X ボタン (GLFW close callback) → ebiten.RunGame が
//     ebiten.Termination を返して graceful 終了
//   - Ctrl+C → Go runtime デフォルト動作 (os.Exit(2)) で即終了
package killswitch

import (
	"os"
	"os/signal"
	"runtime"
	"sync/atomic"
	"syscall"
)

var (
	// triggered は終了トリガが発火したら true になる。
	// atomic.Bool でシグナルハンドラ goroutine と main loop 間の可視性を保証。
	triggered atomic.Bool
)

// Reset はテスト専用。テスト間で triggered 状態をクリアする。
// 本番コードから呼んではならない。
func Reset() {
	triggered.Store(false)
}

// Install は OS シグナルハンドラをインストールする。main 関数から 1 回呼ぶ。
//
// Windows では no-op (上の package doc 参照)。それ以外の OS では
// SIGINT / SIGTERM をリッスンして triggered を立てる。
func Install() {
	if runtime.GOOS == "windows" {
		// Windows では signal.Notify を呼ばない。
		// 詳細は package doc を参照。終了は X ボタンまたは Ctrl+C で
		// 行う (後者は Go runtime デフォルトの即終了)。
		return
	}
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		triggered.Store(true)
	}()
}

// Triggered は終了トリガが発火したかどうかを返す。
// true を返したら Ebitengine.RunGame は ebiten.Termination を返して終了すべき。
//
// Windows では signal.Notify を使っていないため、Triggered() は常に
// false を返す点に注意。Windows の終了は X ボタン (Ebitengine 内部) で
// 直接ハンドリングされる。
func Triggered() bool {
	return triggered.Load()
}
