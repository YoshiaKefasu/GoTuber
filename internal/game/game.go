// Package game は Ebitengine の Game 実装（描画 + 状態管理）を担当する。
// main パッケージから分離し、テスタビリティとモジュール性を向上。
package game

import (
	"image/color"

	"github.com/YoshiaKefasu/GoTuber/internal/character"
	"github.com/YoshiaKefasu/GoTuber/internal/killswitch"
	"github.com/YoshiaKefasu/GoTuber/internal/mouse"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
)

const (
	windowTitle  = "GoTuber"
	windowWidth  = 640
	windowHeight = 480
)

// Game は Ebitengine のゲームロジック実装。
// Phase 1.5: アトラス描画 + マウス追従（Sheet A 固定、表情は Phase 1.6/1.7）
type Game struct {
	atlas       *character.Atlas
	mouse       *mouse.Follower
	firstUpdate bool // Update() 初回呼び出し検出用
}

// New は新しい Game を作成する。
func New(atlas *character.Atlas, follower *mouse.Follower) *Game {
	return &Game{atlas: atlas, mouse: follower, firstUpdate: true}
}

// Update は毎フレーム呼ばれる。
// 初回呼び出し時にクリックスルー + フローティングを有効化（Issue #3222 対策）。
// その後マウス追従を更新し、kill switch をチェックする。
func (g *Game) Update() error {
	if g.firstUpdate {
		g.firstUpdate = false
		// Issue #3222 対策: SetWindowMousePassthrough は Update() 初回内で呼ぶ
		// (RunGame 前に呼ぶと無視される)
		ebiten.SetWindowMousePassthrough(true)
		ebiten.SetWindowFloating(true)
	}

	// マウス位置取得 → Follower 更新
	mx, my := ebiten.CursorPosition()
	g.mouse.Update(mx, my, windowWidth, windowHeight)

	killswitch.Tick()
	if killswitch.Triggered() {
		return ebiten.Termination
	}
	return nil
}

// Draw は画面描画。
//   - アトラス未準備: "Loading..." / "Load error: ..." 表示
//   - 準備完了: マウス位置で決定されるセル（Sheet A 固定）を描画
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

	// マウス追従でセル取得
	// Phase 1.5: Sheet A 固定（目開け + 口閉じ）。Phase 1.6/1.7 で表情切替追加。
	row, col := g.mouse.Cell()
	img, ok := g.atlas.Get(0, row, col)
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
