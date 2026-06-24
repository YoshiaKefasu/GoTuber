// Package game は Ebitengine を使った GoTuber のゲームロジック実装。
package game

import (
	"image/color"
	"math"
	"sync"
	"sync/atomic"
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
	windowTitle         = "GoTuber"
	initialWindowWidth  = 1280
	initialWindowHeight = 720
)

// CameraMode は game 内部で扱う camera mode 値。
//
// Phase 2.8.1: UI 側の magic number を避けるため exported 化する。
type CameraMode int32

// Phase 2.5: camera パッケージとの循環 import / build tag 依存を避ける mode 値。
const (
	CameraModeMouse  CameraMode = 0
	CameraModeCamera CameraMode = 1

	// 内部互換 alias。既存 game ロジックの呼び出し箇所はそのまま維持する。
	cameraModeMouse  int32 = int32(CameraModeMouse)
	cameraModeCamera int32 = int32(CameraModeCamera)
)

// SupervisorCellProvider は camera mode 時の 5x5 atlas cell と瞬き状態を提供する。
//
// Phase 2.5: game パッケージから camera パッケージを import しないための最小 interface。
// `internal/camera/supervisor.go` の *Supervisor がこの interface を満たす。
type SupervisorCellProvider interface {
	CameraCell() (row, col int, ok bool)
	EyesClosed() bool
	MPServerRunning() bool
	LastError() *string
	RestartMPServer() error
}

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

	// Phase 2.5: camera mode。60Hz で supervisor.Mode() から更新される。
	// atomic.Int32 で supervisor loop と game loop 間の lock-free 同期。
	// 値:
	//   0 = CameraModeMouse (Phase 1 既定、mouse.Cell() を使用)
	//   1 = CameraModeCamera (Phase 2、camera mapper を使う予定、Phase 2.5 は placeholder)
	//
	// YAGNI: camera パッケージの CameraMode enum を直接 import せずリテラル定数で
	// 扱う。理由: camera パッケージの build tag 下ファイルを Phase 1 ビルドから
	// 完全に分離するため (YAGNI な依存を排除)。
	//
	// Phase 1 ビルド: ゼロ値 (0 = CameraModeMouse) で固定、既存動作維持。
	// Camera ビルド: camera_hook_camera.go の init() で起動された supervisor loop が
	//               60Hz ごとに SetCameraMode(int(supervisor.Mode())) で更新。
	cameraMode atomic.Int32

	// Phase 2.5: camera mode 時に supervisor から cell / blink を読む。
	// nil のときは mouse follow にフォールバックし、Phase 1 動作を維持する。
	//
	// Phase 2.10.8: camera goroutine (OFF→ON restart) が SetSupervisor で書換えるため、
	// sync.RWMutex で保護。game loop (Update/Draw) は getSupervisor() で
	// ポインタをスナップショットしてから使う (ロック保持中のメソッド呼び出しを回避)。
	supervisorMu sync.RWMutex
	supervisor   SupervisorCellProvider
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
		// Phase 1.14.9: firstUpdate で明示的に passthrough を確定。
		// Phase 1.13a/b (508f630) で applyPassthrough() を導入した際、
		// firstUpdate から SetWindowMousePassthrough(true) の直接呼び出しが消えた結果、
		// Ebitengine v2 + ScreenTransparent:true のデフォルト passthrough=true に
		// 暗黙依存していた。F1 押下後の SetWindowMousePassthrough(false) の効果が
		// Ebitengine v2 GLFW バックエンドで遅延する場合があり、X / 最小化 / 最大化
		// ボタンクリックが通過する症状が出た (Phase 1.14.8 visual test で発覚)。
		// firstUpdate で applyPassthrough() を 1 回呼ぶことで初期状態を確定させる。
		// 起動時 PanelVisible=false (Go ゼロ値) → passthrough=true → Phase 1.2 と同じ初期状態。
		g.applyPassthrough()

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
	// Phase 2.5: camera mode では supervisor の EAR 判定を優先し、mouse mode は
	// Phase 1.6 の blink scheduler を完全維持する。supervisor 未設定時は従来どおり。
	// Phase 2.5.1: 顔未検出 1 秒 grace period 中は CameraCell() の ok=false を見て
	// blink scheduler にフォールバックする。tickCell() は顔未検出フレームで
	// eyesClosed=false を保存するため、EyesClosed() だけを見ると瞬きが 1 秒止まる。
	//
	// Phase 2.10.8: getSupervisor() でスナップショット取得 (camera goroutine との race 防止)。
	sup := g.getSupervisor()
	if g.cameraMode.Load() == cameraModeCamera && sup != nil {
		_, _, ok := sup.CameraCell()
		if ok {
			g.eyesClosed = sup.EyesClosed()
		} else if g.tweaks.BlinkEnabled {
			g.eyesClosed = g.blink.Update(time.Now())
		} else {
			g.eyesClosed = false
		}
	} else if g.tweaks.BlinkEnabled {
		g.eyesClosed = g.blink.Update(time.Now())
	} else {
		g.eyesClosed = false
	}

	// 口パク
	// Phase 1.14.14: Mover.UpdateWithMetrics() の戻り値が Metrics 構造体に変更。
	// RMS / NoiseFloor / GatedRMS / Envelope / Mouth / GateOpen の 6 フィールドを
	// tweaks.State に展開し、Tweaks パネルの 2 行 debug 表示が毎フレーム更新される。
	// Phase 1.14.15: Tweaks パネルの Mic Sensitivity slider の値を Mover に反映。
	// 毎フレーム呼ぶことで slider 変更が即時反映される (無 lock — game.Update は
	// 単一 goroutine からしか呼ばれない)。
	if g.audio != nil && g.tweaks.AudioEnabled {
		g.audio.SetSensitivity(g.tweaks.AudioSensitivity)
		metrics := g.audio.UpdateWithMetrics()
		g.mouthState = metrics.Mouth
		g.tweaks.AudioRMS = metrics.RMS
		g.tweaks.AudioNoiseFloor = metrics.NoiseFloor
		g.tweaks.AudioGatedRMS = metrics.GatedRMS
		g.tweaks.AudioEnvelope = metrics.Envelope
		g.tweaks.AudioMouthState = metrics.Mouth
		g.tweaks.AudioGateOpen = metrics.GateOpen
	} else {
		g.mouthState = 0
		g.tweaks.AudioRMS = 0
		g.tweaks.AudioNoiseFloor = 0
		g.tweaks.AudioGatedRMS = 0
		g.tweaks.AudioEnvelope = 0
		g.tweaks.AudioMouthState = 0
		g.tweaks.AudioGateOpen = false
	}

	// Tweaks panel UI 更新
	if g.panel != nil {
		if sup != nil {
			g.panel.UpdateCameraStatus(
				int(g.cameraMode.Load()),
				sup.MPServerRunning(),
				sup.LastError(),
				g.IsCameraEnabled(),
			)
		} else {
			g.panel.UpdateCameraStatus(int(g.cameraMode.Load()), false, nil, g.IsCameraEnabled())
		}
	}
	if g.tweaks.PanelVisible {
		g.panel.Update()
	}

	// SIGINT/SIGTERM 終了検出 (Unix only、Phase 1.14 で Esc 検出削除)
	// Windows では Install() が no-op のため Triggered() は常に false。
	// 終了はウィンドウ X ボタン (GLFW close callback) または Tweaks Quit ボタン。
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

	row, col := g.currentCell()
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
	// FilterLinear: スケール時のジャギーを防ぐ (Ebitengine デフォルトの FilterNearest だと
	// 1200x1200 スプライトを 720x720 (1280x720 ウィンドウ中央に letterbox) に縮小した時に
	// エッジがギザギザになる)。縮小方向は Linear で十分、拡大方向は Pixel-art なら Nearest の
	// ほうが好ましいが現実装は 5x5 cell (各 1200x1200) を画面サイズに縮小する用途が主なので
	// Linear で統一。
	op.Filter = ebiten.FilterLinear
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(ox, oy)
	screen.DrawImage(img, op)

	if !g.uiHidden && g.tweaks.PanelVisible {
		g.panel.Draw(screen)
	}
}

// currentCell は現在の camera mode に応じた atlas cell を返す。
//
// Phase 2.5: camera mode かつ supervisor が有効な場合だけ camera cell を使う。
// supervisor が未起動 / 顔未検出 / 最新 cell なしの場合は Phase 1.12 の mouse.Cell() に
// フォールバックし、既存 mouse follow ロジックを変更しない。
func (g *Game) currentCell() (row, col int) {
	sup := g.getSupervisor()
	if g.cameraMode.Load() == cameraModeCamera && sup != nil {
		row, col, ok := sup.CameraCell()
		if ok {
			return row, col
		}
	}
	return g.mouse.Cell()
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

// passthroughDesired は X ボタンを常に有効化するため常に false を返す。
// 純粋関数なのでユニットテストでカバー可能 (ebiten context 不要)。
//
// Phase 1.14.10: passthrough 全面廃止。
// Phase 1.14.9 firstUpdate fix 後も F1 パネル非表示時に X ボタンが通過する問題が
// yosia さん実機 visual test で発覚。SetWindowMousePassthrough(true) は Windows の
// WS_EX_TRANSPARENT 拡張スタイルを設定し、ウィンドウ全体 (タイトルバー含む) が
// クリック透過になる。Ebitengine v2.9.9 純粋 API では per-region passthrough は
// 不可能で、Win32 API (CGo) や FramelessWindow 自前タイトルバー描画は工数大。
//
// 最小修正として passthrough を全面廃止。犠牲: OBS クリック透過 (キャラ部分クリック
// が背後のウィンドウに届かない)。ScreenTransparent: true は維持するため背景透過は
// OK。Phase 2+ で Win32 API または自前タイトルバーで per-region passthrough を
// 復活する時のアンカーとして applyPassthrough と firstUpdate 呼び出しは残す。
func passthroughDesired(panelVisible, uiHidden bool) bool {
	return false
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

// SetCameraMode は camera mode を設定する (Phase 2.5)。
//
// camera_hook_camera.go (`//go:build camera`) の init() で起動された supervisor loop が
// 60Hz ごとに supervisor.Mode() の値を読み取ってこのメソッドを呼ぶ。
// Phase 1 ビルドでは呼ばれない (cameraHook が nil、runCameraHook が no-op)。
//
// 引数:
//
//	mode — 0 = CameraModeMouse (Phase 1 既定)、1 = CameraModeCamera (Phase 2)
//
// 実装メモ: 内部状態 cameraMode は atomic.Int32 なので lock-free 読み出し可能、
//
//	game.Update() の 60Hz hot path から mutex なしで参照できる。
//
// YAGNI: camera パッケージの enum を import せず int で扱う。Phase 1 ビルドでも
//
//	メソッド自体はビルドに含まれるが、camera_hook_camera.go (`//go:build camera`)
//	からのみ呼ばれるので Phase 1 動作には影響しない。
func (g *Game) SetCameraMode(mode int) {
	g.cameraMode.Store(int32(mode))
}

// SetSupervisor は camera mode 用の cell provider を設定する (Phase 2.5)。
//
// camera_hook_camera.go (`//go:build camera`) から *camera.Supervisor を渡す。
// Phase 1 ビルドでは呼ばれず、nil のまま mouse follow が動作する。
//
// Phase 2.10.8: camera goroutine の OFF→ON restart から呼ばれるため、
// 書き込みロックで保護する。
func (g *Game) SetSupervisor(supervisor SupervisorCellProvider) {
	g.supervisorMu.Lock()
	g.supervisor = supervisor
	g.supervisorMu.Unlock()
}

// getSupervisor は supervisor ポインタの安全なスナップショットを返す (Phase 2.10.8)。
//
// 読み取りロックでポインタだけコピーし、ロックをすぐ解放する。
// 呼び出し側は戻り値 sup を使い、g.supervisor を直接触らない。
// nil のときは nil が返る (Phase 1 ビルド / camera 停止中)。
func (g *Game) getSupervisor() SupervisorCellProvider {
	g.supervisorMu.RLock()
	sup := g.supervisor
	g.supervisorMu.RUnlock()
	return sup
}

// RestartCamera は Tweaks panel の Manual Restart ボタンから camera supervisor へ再起動を委譲する。
//
// Phase 2.8: game パッケージは camera パッケージを import しないため、interface 経由で呼ぶ。
// Phase 2.10.8: CameraEnabled=false のときは no-op (カメラが停止しているため再起動不要)。
func (g *Game) RestartCamera() error {
	sup := g.getSupervisor()
	if sup == nil {
		return nil
	}
	if !g.IsCameraEnabled() {
		return nil // Camera OFF: ユーザーが OFF にした以上再起動不要
	}
	return sup.RestartMPServer()
}

// IsCameraEnabled は Tweaks の CameraEnabled フィールドを返す (Phase 2.10.8)。
//
// camera_hook_camera.go の supervisor mode 反映 goroutine で、
// CameraEnabled=false のとき CameraModeMouse (0) を強制するために使う。
func (g *Game) IsCameraEnabled() bool {
	if g.tweaks == nil {
		return true // nil はデフォルト ON
	}
	return g.tweaks.CameraEnabled
}
