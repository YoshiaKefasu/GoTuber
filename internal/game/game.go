// Package game は Ebitengine を使った GoTuber のゲームロジック実装。
package game

import (
	"image/color"
	"math"
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
	windowTitle             = "GoTuber"
	initialWindowWidth      = 1280
	initialWindowHeight     = 720
)

// DeviceListMessage は起動時バックグラウンドで列挙したデバイス一覧を
// メインスレッド (Game.Update) に通知するためのチャネルメッセージ (Phase 1.13a S-1)。
//
// ebitenui は goroutine safe ではないため、起動時バックグラウンド goroutine からは
// 直接 panel.SetDevices() を呼べない。代わりに chan DeviceListMessage に送信し、
// Game.Update 内の select でメインスレッドに dispatch する。
type DeviceListMessage struct {
	Devices []audio.Device
}

// Game は Ebitengine のゲームロジック実装。
type Game struct {
	atlas  *character.Atlas
	mouse  *mouse.Follower
	blink  *blink.Scheduler
	audio  *audio.Mover
	panel  *tweaks.Panel
	tweaks *tweaks.State

	firstUpdate bool

	// 現在のウィンドウサイズ (Layout() で毎フレーム更新)
	width  int
	height int

	// 内部状態
	eyesClosed bool
	mouthState int

	// Phase 1.13b: UI 全非表示フラグ。Ctrl+Shift+H で toggle。
	// true のとき Tweaks パネル + 設定 UI (将来追加分) 全部を非表示。
	// kill switch (Esc/SIGINT) は uiHidden に関わらず常に有効。
	uiHidden bool

	// Phase 1.13a S-1: 起動時バックグラウンド goroutine から
	// デバイス一覧を受け取るチャネル (buffered, 容量 1)。
	// nil のときは select で default に流れる (テスト用最小初期化でも安全)。
	devicesCh chan DeviceListMessage
}

// New は新しい Game を作成する。
// devicesCh: 起動時バックグラウンド goroutine から device list を受け取るチャネル。
//
// nil を渡すと dispatch ロジックは無効化される (テスト時に便利)。
// main.go では必ず make(chan DeviceListMessage, 1) を渡す。
func New(
	atlas *character.Atlas,
	follower *mouse.Follower,
	blinkSch *blink.Scheduler,
	audioMover *audio.Mover,
	panel *tweaks.Panel,
	tweaksState *tweaks.State,
	devicesCh chan DeviceListMessage,
) *Game {
	return &Game{
		atlas:       atlas,
		mouse:       follower,
		blink:       blinkSch,
		audio:       audioMover,
		panel:       panel,
		tweaks:      tweaksState,
		firstUpdate: true,
		width:       initialWindowWidth,
		height:      initialWindowHeight,
		uiHidden:    false, // explicit: 初期は全 UI 表示状態 (F1 で開く)
		devicesCh:   devicesCh,
	}
}

// Update は毎フレーム呼ばれる。
func (g *Game) Update() error {
	if g.firstUpdate {
		g.firstUpdate = false
		// 透過ウィンドウの Z-Order は cmd/gotuber/main.go で
		// ebiten.SetWindowFloating(*flagTopmost) を --topmost フラグ (default: false) により
		// 制御する。Ebitengine v2 の透過ウィンドウは OS 仕様で Z-Order が上位に来るため。
	}

	// F1 で panel 表示切替 (単発検出)
	prevPanelVisible := g.tweaks.PanelVisible
	if inpututil.IsKeyJustPressed(ebiten.KeyF1) {
		g.tweaks.PanelVisible = !g.tweaks.PanelVisible
	}
	// パネル表示状態に応じてクリックスルー切替 (Issue #3222 対策で Update 内で呼ぶ)
	// PanelVisible 中はマウスイベントをウィンドウに届ける必要があるため passthrough 無効化
	// uiHidden=true のときは PanelVisible に関わらず passthrough=true にする
	// (1.13b: 全 UI 非表示フラグ、Ctrl+Shift+H で toggle)
	if g.tweaks.PanelVisible != prevPanelVisible {
		g.applyPassthrough()
	}

	// Phase 1.13b: Ctrl+Shift+H で全 UI 非表示トグル (配信時 OBS キャプチャ対策)
	// P-1: 2 キーは IsKeyPressed (押下状態) + 1 キーは IsKeyJustPressed (立ち上がりエッジ)。
	//      3 キー同時 "just pressed" は物理的に検出できない。
	if ebiten.IsKeyPressed(ebiten.KeyControl) && ebiten.IsKeyPressed(ebiten.KeyShift) && inpututil.IsKeyJustPressed(ebiten.KeyH) {
		g.ToggleUIHidden()
	}

	// Phase 1.13a S-1: 起動時バックグラウンド goroutine からの panel.SetDevices を
	// メインスレッドに dispatch する。ebitenui は goroutine safe ではないため。
	// バッファ付きチャネル + default ケースで、フレーム予算を消費しない (non-blocking)。
	// nil チャネル (テスト時) は select で default に流れるので安全。
	select {
	case msg := <-g.devicesCh:
		if g.panel != nil {
			g.panel.SetDevices(msg.Devices)
		}
	default:
	}

	// Tweaks panel の Quit ボタン
	if g.tweaks.QuitRequested {
		return ebiten.Termination
	}

	// マウス追従
	mx, my := ebiten.CursorPosition()
	g.mouse.Update(mx, my, g.width, g.height, g.tweaks.MouseResponsiveness)

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
	// アスペクト比を維持してウィンドウ内に収まるようスケール。
	// スプライト 1200x1200 をウィンドウサイズに合わせる。
	scaleX := float64(g.width) / float64(iw)
	scaleY := float64(g.height) / float64(ih)
	scale := math.Min(scaleX, scaleY)
	scaledW := float64(iw) * scale
	scaledH := float64(ih) * scale
	ox := (float64(g.width) - scaledW) / 2
	oy := (float64(g.height) - scaledH) / 2
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(ox, oy)
	screen.DrawImage(img, op)

	if !g.uiHidden && g.tweaks.PanelVisible {
		g.panel.Draw(screen)
	}
}

// sheetForState は現在の (eyesState, mouthState) に対応する sheet index を返す。
// character.Config.SheetFor (元 character-config.js の sheets マッピング) に委譲。
func (g *Game) sheetForState() int {
	_, idx := g.atlas.SheetFor(g.eyesClosed, g.mouthState)
	return idx
}

// Layout はウィンドウサイズを返す。
// SetWindowResizingMode(WindowResizingModeEnabled) でリサイズ可能なため、
// ユーザー操作でウィンドウサイズが変わると Ebitengine がこの関数を呼ぶ。
// 内部キャンバスはウィンドウサイズに追従する。
func (g *Game) Layout(w, h int) (int, int) {
	g.width = w
	g.height = h
	return w, h
}

// WindowTitle は Ebitengine の SetWindowTitle に渡す定数。
func WindowTitle() string { return windowTitle }

// passthroughDesired は UI 表示状態から期待されるクリックスルー (passthrough) を返す。
// 純粋関数なのでユニットテストでカバー可能 (ebiten context 不要)。
//
// 真理値表:
//   - panelVisible=true,  uiHidden=false → passthrough=false (UI クリック受付)
//   - panelVisible=true,  uiHidden=true  → passthrough=true  (UI を全部隠す)
//   - panelVisible=false, uiHidden=true  → passthrough=true  (UI を全部隠す)
//   - panelVisible=false, uiHidden=false → passthrough=true  (UI は元々非表示)
func passthroughDesired(panelVisible, uiHidden bool) bool {
	return !(panelVisible && !uiHidden)
}

// applyPassthrough は UI 表示状態に応じてクリックスルーを切り替える。
// PanelVisible トグル時・uiHidden トグル時の両方から呼ばれる集約点。
// g.tweaks が nil (テスト用最小初期化) のときは何もしない。
func (g *Game) applyPassthrough() {
	if g.tweaks == nil {
		return
	}
	ebiten.SetWindowMousePassthrough(passthroughDesired(g.tweaks.PanelVisible, g.uiHidden))
}

// ToggleUIHidden は uiHidden フラグを反転し、Panel に通知 + passthrough を更新する。
// Update() のキー検出からも、テストからも呼ばれる公開メソッド。
func (g *Game) ToggleUIHidden() {
	g.uiHidden = !g.uiHidden
	if g.panel != nil {
		g.panel.SetUIHidden(g.uiHidden)
	}
	g.applyPassthrough()
}
