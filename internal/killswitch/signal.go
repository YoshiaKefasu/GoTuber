// Package killswitch provides graceful shutdown via SIGINT, SIGTERM, and Esc key.
//
// 3 つの終了トリガ:
//  1. SIGINT (Ctrl+C) — Unix 標準、Windows も同等
//  2. SIGTERM — Unix 標準
//  3. Esc キー — Ebitengine メインループから Tick() で検出
//
// Ebitengine の inpututil は goroutine から安全でないため、Esc 検出は
// Update() ループ内（= Tick() 呼び出し）で同期的に行う。
package killswitch

import (
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

var (
	escPressed atomic.Bool // Esc キーが押されたら true
	triggered  atomic.Bool // 終了トリガが発火したら true
)

// Reset はテスト専用。テスト間で状態 (escPressed, triggered) をクリアする。
// 本番コードから呼んではならない。
func Reset() {
	escPressed.Store(false)
	triggered.Store(false)
}

// Install は OS シグナルハンドラをインストールする。main 関数から 1 回呼ぶ。
func Install() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		triggered.Store(true)
	}()
}

// Tick は Ebitengine の Update() から毎フレーム呼ばれる。
// inpututil は main loop 外から呼ぶと unsafe なため、ここで同期的に Esc を検出する。
// 他の goroutine から呼んではならない。
func Tick() {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		escPressed.Store(true)
	}
}

// Triggered は終了トリガが発火したかどうかを返す。
// true を返したら Ebitengine.RunGame は ebiten.Termination を返して終了すべき。
func Triggered() bool {
	return triggered.Load() || escPressed.Load()
}
