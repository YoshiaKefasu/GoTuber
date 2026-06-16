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
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
)

const (
	windowTitle  = "GoTuber"
	windowWidth  = 640
	windowHeight = 480
)

// Game は Ebitengine のゲームロジック実装。
type Game struct {
	atlas       *character.Atlas
	mouse       *mouse.Follower
	blink       *blink.Scheduler
	audio       *audio.Mover // nil 可（オーディオデバイスなし環境用）
	firstUpdate bool

	// 内部状態 (Update で更新、Draw で参照)
	eyesClosed bool // blink scheduler から
	mouthState int  // 0=closed, 1=half, 2=open (audio.Mover から)
}

// New は新しい Game を作成する。audioMover が nil でも動作する（口パク無効）。
func New(atlas *character.Atlas, follower *mouse.Follower, blinkSch *blink.Scheduler, audioMover *audio.Mover) *Game {
	return &Game{atlas: atlas, mouse: follower, blink: blinkSch, audio: audioMover, firstUpdate: true}
}

// Update は毎フレーム呼ばれる。
func (g *Game) Update() error {
	if g.firstUpdate {
		g.firstUpdate = false
		ebiten.SetWindowMousePassthrough(true)
		ebiten.SetWindowFloating(true)
	}

	// マウス位置取得 → Follower 更新
	mx, my := ebiten.CursorPosition()
	g.mouse.Update(mx, my, windowWidth, windowHeight)

	// 自動まばたき更新
	g.eyesClosed = g.blink.Update(time.Now())

	// 口パク更新（audio デバイスがない場合はスキップ）
	if g.audio != nil {
		g.mouthState = g.audio.Update()
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
		return
	}

	row, col := g.mouse.Cell()
	sheetIdx := g.sheetForState()
	img, ok := g.atlas.Get(sheetIdx, row, col)
	if !ok || img == nil {
		return
	}

	// 中央描画
	iw, ih := img.Bounds().Dx(), img.Bounds().Dy()
	ox := (windowWidth - iw) / 2
	oy := (windowHeight - ih) / 2
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(ox), float64(oy))
	screen.DrawImage(img, op)
}

// sheetForState は現在の (eyesState, mouthState) に対応する sheet index を返す。
// 戻り値:
//   - 0: eyes_open + mouth_closed
//   - 1: eyes_open + mouth_half
//   - 2: eyes_open + mouth_open
//   - 3: eyes_closed + mouth_closed
//   - 4: eyes_closed + mouth_half
//   - 5: eyes_closed + mouth_open
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
