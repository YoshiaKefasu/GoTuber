// Package main is the entry point of GoTuber.
//
// Phase 1.1: 最小 main.go — 空の Ebitengine ウィンドウ + kill switch。
// カメラ・マイク・透過・クリックスルーは次フェーズで追加。
package main

import (
	"log"
	"os"

	"github.com/YoshiaKefasu/GoTuber/internal/killswitch"
	"github.com/hajimehoshi/ebiten/v2"
)

const (
	windowTitle  = "GoTuber"
	windowWidth  = 640
	windowHeight = 480
)

// Game は Ebitengine のゲームロジック実装。Phase 1.1 では kill switch のみ。
type Game struct{}

// Update は毎フレーム呼ばれる。Esc キーや SIGINT をチェックする。
func (g *Game) Update() error {
	killswitch.Tick()
	if killswitch.Triggered() {
		return ebiten.Termination
	}
	return nil
}

// Draw は画面描画。Phase 1.1 では何もしない（白画面）。
func (g *Game) Draw(screen *ebiten.Image) {
	// TODO: アバター描画は Phase 1.3 以降で実装
}

// Layout はウィンドウサイズを返す。
func (g *Game) Layout(w, h int) (int, int) {
	return windowWidth, windowHeight
}

func main() {
	// OS シグナルハンドラ（SIGINT / SIGTERM）をインストール
	killswitch.Install()

	// ウィンドウ設定
	ebiten.SetWindowTitle(windowTitle)
	ebiten.SetWindowSize(windowWidth, windowHeight)

	// ゲームループ開始
	if err := ebiten.RunGame(&Game{}); err != nil {
		log.Printf("GoTuber terminated: %v", err)
		os.Exit(0)
	}
}
