// Package game は Ebitengine の Game 実装（描画 + 状態管理）を担当する。
// main パッケージから分離し、テスタビリティとモジュール性を向上。
package game

import (
	"image/color"

	"github.com/YoshiaKefasu/GoTuber/internal/character"
	"github.com/YoshiaKefasu/GoTuber/internal/killswitch"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
)

const (
	windowTitle  = "GoTuber"
	windowWidth  = 640
	windowHeight = 480
)

// Game は Ebitengine のゲームロジック実装。
// Phase 1.3: アトラス読み込み中表示 + 準備完了後のデフォルトセル描画。
type Game struct {
	atlas       *character.Atlas
	firstUpdate bool // Update() 初回呼び出し検出用
}

// New は新しい Game を作成する。
func New(atlas *character.Atlas) *Game {
	return &Game{atlas: atlas, firstUpdate: true}
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

// Draw は画面描画。Phase 1.3 では:
//   - アトラス未準備 & エラーなし: "Loading..." テキスト表示（半透明黒背景）
//   - アトラス未準備 & エラーあり: エラーメッセージ表示（半透明赤背景）
//   - 準備完了: デフォルトセル（Sheet A, r2c2 = 中央）を表示
func (g *Game) Draw(screen *ebiten.Image) {
	// アトラス未準備時の状態（Loading / Error）
	if !g.atlas.Ready() {
		// ebitenutil.DebugPrint の白文字は透過背景で見えないため、背景を自前描画
		if err := g.atlas.LastErr(); err != nil {
			screen.Fill(color.RGBA{128, 0, 0, 200}) // 半透明赤
			ebitenutil.DebugPrintAt(screen, "Load error: "+err.Error(), 20, 20)
		} else {
			screen.Fill(color.RGBA{0, 0, 0, 180}) // 半透明黒
			ebitenutil.DebugPrintAt(screen, "Loading...", 20, 20)
		}
		return
	}

	// デフォルトセルを描画（Phase 1.3 では中央固定）
	// Phase 1.5 で mouse follow に置換予定
	img, ok := g.atlas.Get(0, 2, 2) // Sheet A, row 2, col 2
	if !ok || img == nil {
		return
	}

	// ウィンドウ中央に描画
	iw, ih := img.Bounds().Dx(), img.Bounds().Dy()
	ox := (windowWidth - iw) / 2
	oy := (windowHeight - ih) / 2
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(ox), float64(oy))
	screen.DrawImage(img, op)
}

// Layout はウィンドウサイズを返す。
func (g *Game) Layout(w, h int) (int, int) {
	return windowWidth, windowHeight
}

// WindowTitle は Ebitengine の SetWindowTitle に渡す定数。
func WindowTitle() string { return windowTitle }
