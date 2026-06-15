// Package main is the entry point of GoTuber.
//
// Phase 1.2: 透過ウィンドウ + クリックスルー。
// カメラ・マイク・アバター描画は次フェーズ。
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

// Game は Ebitengine のゲームロジック実装。
type Game struct {
	firstUpdate bool // Update() の初回呼び出し検出用
}

// Update は毎フレーム呼ばれる。
// 初回呼び出し時にクリックスルー + フローティングを有効化（Issue #3222 対策）。
// その後 kill switch をチェックする。
func (g *Game) Update() error {
	if g.firstUpdate {
		g.firstUpdate = false
		// Issue #3222 対策: SetWindowMousePassthrough は Update() 初回内で呼ぶ
		// (RunGame 前に呼ぶと無視される)
		ebiten.SetWindowMousePassthrough(true)
		ebiten.SetWindowFloating(true)
	}

	killswitch.Tick()
	if killswitch.Triggered() {
		return ebiten.Termination
	}
	return nil
}

// Draw は画面描画。Phase 1.2 では何もしない（透過白画面）。
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

	// 透過背景 + クリックスルー有効化
	//   - ScreenTransparent は RunGame 前に設定（透過背景）
	//   - SetWindowMousePassthrough は Update() 初回内で設定（クリックスルー）
	//   - Issue #3222 回避のためクリックスルーは RunGame 後に呼ぶ
	opts := &ebiten.RunGameOptions{
		ScreenTransparent: true,
	}

	game := &Game{firstUpdate: true}

	// ゲームループ開始
	// ebiten.Termination は kill switch 発火時の正常終了として扱う（終了コード 0）
	if err := ebiten.RunGameWithOptions(game, opts); err != nil && err != ebiten.Termination {
		log.Printf("GoTuber terminated with error: %v", err)
		os.Exit(1)
	}
}
