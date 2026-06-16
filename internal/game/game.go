// Package game は Ebitengine を使った GoTuber のゲームロジック実装。
package game

import (
	"image/color"
	"time"

	"github.com/YoshiaKefasu/GoTuber/internal/audio"
	"github.com/YoshiaKefasu/GoTuber/internal/blink"
	"github.com/YoshiaKefasu/GoTuber/internal/character"
	"github.com/YoshiaKefasu/GoTuber/internal/killswitch"
	"github.com/YoshiaKefasu/GoTuber/internal/mouse"
	"github.com/YoshiaKefasu/GoTuber/internal/tweaks"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

const (
	windowTitle  = "GoTuber"
	windowWidth  = 640
	windowHeight = 480
)

// Game は Ebitengine のゲームロジック実装。
type Game struct {
	atlas  *character.Atlas
	mouse  *mouse.Follower
	blink  *blink.Scheduler
	audio  *audio.Mover
	panel  *tweaks.Panel
	tweaks *tweaks.State

	firstUpdate bool

	// 内部状態
	eyesClosed bool
	mouthState int
}

// New は新しい Game を作成する。
func New(
	atlas *character.Atlas,
	follower *mouse.Follower,
	blinkSch *blink.Scheduler,
	audioMover *audio.Mover,
	panel *tweaks.Panel,
	tweaksState *tweaks.State,
) *Game {
	return &Game{
		atlas:       atlas,
		mouse:       follower,
		blink:       blinkSch,
		audio:       audioMover,
		panel:       panel,
		tweaks:      tweaksState,
		firstUpdate: true,
	}
}

// Update は毎フレーム呼ばれる。
func (g *Game) Update() error {
	if g.firstUpdate {
		g.firstUpdate = false
		ebiten.SetWindowFloating(true)
	}

	// F1 で panel 表示切替 (単発検出)
	prevPanelVisible := g.tweaks.PanelVisible
	if inpututil.IsKeyJustPressed(ebiten.KeyF1) {
		g.tweaks.PanelVisible = !g.tweaks.PanelVisible
	}
	// パネル表示状態に応じてクリックスルー切替 (Issue #3222 対策で Update 内で呼ぶ)
	// PanelVisible 中はマウスイベントをウィンドウに届ける必要があるため passthrough 無効化
	if g.tweaks.PanelVisible != prevPanelVisible {
		ebiten.SetWindowMousePassthrough(!g.tweaks.PanelVisible)
	}

	// Tweaks panel の Quit ボタン
	if g.tweaks.QuitRequested {
		return ebiten.Termination
	}

	// マウス追従
	mx, my := ebiten.CursorPosition()
	g.mouse.Update(mx, my, windowWidth, windowHeight, g.tweaks.MouseResponsiveness)

	// 自動まばたき
	if g.tweaks.BlinkEnabled {
		g.eyesClosed = g.blink.Update(time.Now())
	} else {
		g.eyesClosed = false
	}

	// 口パク
	if g.audio != nil && g.tweaks.AudioEnabled {
		g.mouthState = g.audio.Update()
	} else {
		g.mouthState = 0
	}

	// Tweaks panel UI 更新
	if g.tweaks.PanelVisible {
		g.panel.Update()
	}

	// kill switch
	killswitch.Tick()
	if killswitch.Triggered() {
		return ebiten.Termination
	}
	return nil
}

// Draw は画面描画。
func (g *Game) Draw(screen *ebiten.Image) {
	if !g.atlas.Ready() {
		if err := g.atlas.LastErr(); err != nil {
			screen.Fill(color.RGBA{128, 0, 0, 200})
			ebitenutil.DebugPrintAt(screen, "Load error: "+err.Error(), 20, 20)
		} else {
			screen.Fill(color.RGBA{0, 0, 0, 180})
			ebitenutil.DebugPrintAt(screen, "Loading...", 20, 20)
		}
		if g.tweaks.PanelVisible {
			g.panel.Draw(screen)
		}
		return
	}

	row, col := g.mouse.Cell()
	sheetIdx := g.sheetForState()
	img, ok := g.atlas.Get(sheetIdx, row, col)
	if !ok || img == nil {
		return
	}

	iw, ih := img.Bounds().Dx(), img.Bounds().Dy()
	ox := (windowWidth - iw) / 2
	oy := (windowHeight - ih) / 2
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(ox), float64(oy))
	screen.DrawImage(img, op)

	if g.tweaks.PanelVisible {
		g.panel.Draw(screen)
	}
}

// sheetForState は現在の (eyesState, mouthState) に対応する sheet index を返す。
func (g *Game) sheetForState() int {
	eyesIdx := 0
	if g.eyesClosed {
		eyesIdx = 1
	}
	return eyesIdx*3 + g.mouthState
}

// Layout はウィンドウサイズを返す。
func (g *Game) Layout(w, h int) (int, int) {
	return windowWidth, windowHeight
}

// WindowTitle は Ebitengine の SetWindowTitle に渡す定数。
func WindowTitle() string { return windowTitle }
