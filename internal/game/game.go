// Package game は Ebitengine を使った GoTuber のゲームロジック実装。
package game

import (
	"fmt"
	"image"
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
	"github.com/YoshiaKefasu/GoTuber/internal/render"
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

// ─── Phase 4.0: Cell Transition α-blend ───────────────────────────────────

// cellTransition はセル切り替え時の α ブレンド遷移状態を保持する。
//
// 表示セル (sheetIdx, row, col) が変わったとき、旧セルから新セルへ
// 一定時間 (default 100ms) かけてフェードクロスする。
// 純粋関数 updateTransition で状態を進めるため、ユニットテストで検証可能。
type cellTransition struct {
	fromSheet, fromRow, fromCol int   // フェードアウト中の旧セル
	progress                    float64 // 0.0 = 旧セル表示, 1.0 = 新セル表示
	active                      bool    // true = 遷移中
}

// updateTransition はセル遷移状態を 1 フレーム分進める純粋関数。
//
// 引数:
//   - t:          現在の遷移状態
//   - prev*:      前フレームの表示セル (変化検出用)
//   - cur*:       今フレームの表示セル
//   - deltaSec:   前フレームからの経過秒数
//   - durationSec: 遷移総期間 (秒)
//
// 返り値: 更新された遷移状態。
//
// 動作:
//   - 非遷移中にセルが変わった → 遷移開始 (from = prev)
//   - 遷移中にセルが変わった → 遷移再開 (from = prev, progress=0)
//     (prev は前フレームで更新済み = 直近の遷移先セル)
//   - 期間到達 → 遷移終了 (active=false)
func updateTransition(
	t cellTransition,
	prevSheet, prevRow, prevCol int,
	curSheet, curRow, curCol int,
	deltaSec, durationSec float64,
) cellTransition {
	cellChanged := curSheet != prevSheet || curRow != prevRow || curCol != prevCol

	if !t.active {
		if cellChanged {
			return cellTransition{
				fromSheet: prevSheet, fromRow: prevRow, fromCol: prevCol,
				progress: 0, active: true,
			}
		}
		return t
	}

	// 遷移中: 進行度を進める
	t.progress += deltaSec / durationSec

	if cellChanged {
		// 遷移中にセルが変わった → 現在の遷移先を起点に再開
		return cellTransition{
			fromSheet: prevSheet, fromRow: prevRow, fromCol: prevCol,
			progress: 0, active: true,
		}
	}

	if t.progress >= 1.0 {
		t.progress = 1.0
		t.active = false
		return t
	}

	return t
}

// ─── Game ──────────────────────────────────────────────────────────────────

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

	// ─── Phase 4.0: Cell Transition α-blend ───────────────────────────────

	// transEnabled はセル切り替え時の α ブレンド遷移を有効にするか。
	// デフォルト true。Tweaks UI からの切替は Phase 4.3 で実装済み。
	// ここを false にすると従来通りの即時切り替えに戻る。
	transEnabled bool

	// transDuration は遷移期間 (秒)。PHASE4.md 仕様値 100ms = 0.1s。
	// Phase 4.3: Tweaks UI で 50〜200ms 範囲で調整可能。毎フレーム state から反映。
	transDuration float64

	// trans は現在の遷移状態。updateTransition() で毎フレーム更新。
	trans cellTransition

	// prevSheet/Row/Col は前フレームの表示セル (変化検出用)。
	// Update() 内で毎フレーム currentCell()/sheetForState() の結果を保存する。
	// firstCellSet=false の初回だけは遷移を開始せず、現在セルをそのまま初期値として採用する。
	prevSheet int
	prevRow   int
	prevCol   int
	firstCellSet bool

	// lastDrawTime は前フレームの時刻。updateTransition への deltaSec 計算に使う。
	lastDrawTime    time.Time
	firstDrawPassed bool // 初回 Update で lastDrawTime を初期化済みか

	// ─── Phase 4.1: Mesh Renderer ──────────────────────────────────────────

	// meshCache は画像サイズごとのメッシュをキャッシュする。
	// 同じ画像サイズの描画が繰り返されるため、毎フレーム再生成しない。
	// キー: "imgW×imgH@screenW×screenH"、値: フラットメッシュ
	meshCacheMu sync.RWMutex
	meshCache   map[string]*render.MeshGrid

	// ─── Phase 4.2: Depth-weighted Elastic Morph ──────────────────────────

	// morphElastic は elastic morph の滑らか追従状態。
	// 毎フレーム mouse current から target を計算し、EMA で追従。
	morphElastic render.MorphElastic

	// morphDepthPath は現在のセルに対応する depth map パス。
	// セルが変わったときだけ再計算。空なら flat mesh fallback。
	morphDepthPath string

	// ─── Phase 4.4: Performance tuning / fallback ──────────────────────

	// morphMeshCache は elastic 変位を量子化したキーで morphed mesh をキャッシュする。
	// キー: "imgW×imgH@screenW×screenH#qElX×qElY#strength#depthPath"。
	// 量子化: elastic 変位を 0.5px 刻みに丸め、近いフレームで再利用する。
	morphMeshCacheMu sync.RWMutex
	morphMeshCache   map[string]*render.MeshGrid

	// fpsTracking は FPS 自動 fallback のための測定状態。
	fpsFrameCount   int
	fpsLastCheck    time.Time
	fpsCurrentAvg   float64 // 直近 1 秒の平均 FPS
	morphAutoDisable bool   // FPS 低下時に一時的に morph を無効化
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
		atlas:          atlas,
		mouse:          follower,
		blink:          blinkSch,
		audio:          audioMover,
		panel:          panel,
		tweaks:         tweaksState,
		firstUpdate:    true,
		width:          initialWindowWidth,
		height:         initialWindowHeight,
		uiHidden:       false, // explicit: 初期は全 UI 表示状態 (F1 で開く)
		devicesCh:      devicesCh,
		transEnabled:   true,                           // Phase 4.0: α-blend 有効 (default)
		transDuration:  0.1,                            // 100ms (PHASE4.md 仕様値)
		prevSheet:      -1,
		prevRow:        -1,
		prevCol:        -1,
		firstCellSet:   false,
		firstDrawPassed: false,
		meshCache:      make(map[string]*render.MeshGrid),
		morphMeshCache: make(map[string]*render.MeshGrid),
		fpsLastCheck:   time.Now(),
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

	// Phase 4.0: セル遷移 α-blend — セル変化検出 + 進行度更新
	// eyesClosed / mouthState が確定した後に計算する。
	// Phase 4.3: Tweaks UI の TransitionDuration (ms) を毎フレーム反映。
	g.transDuration = g.tweaks.TransitionDuration / 1000.0 // ms → sec
	if g.transDuration < 0.01 {
		g.transDuration = 0.01 // 最小 10ms
	}
	if g.transEnabled {
		now := time.Now()
		var deltaSec float64
		if g.firstDrawPassed {
			deltaSec = now.Sub(g.lastDrawTime).Seconds()
		}
		g.lastDrawTime = now
		g.firstDrawPassed = true

		_, curSheet := g.sheetForState()
		curRow, curCol := g.currentCell()

		if !g.firstCellSet {
			// 起動直後の 1 フレーム目は、旧セルが存在しない。
			// ここで遷移を開始すると from=(-1,-1,-1) になり、
			// Draw() 側で旧セル nil / 新セル alpha=0 となって 100ms フェードインしてしまう。
			// 初回だけは現在セルをそのまま採用し、遷移を開始しない。
			g.firstCellSet = true
		} else {
			g.trans = updateTransition(g.trans,
				g.prevSheet, g.prevRow, g.prevCol,
				curSheet, curRow, curCol,
				deltaSec, g.transDuration)
		}

		g.prevSheet = curSheet
		g.prevRow = curRow
		g.prevCol = curCol
	}

	// SIGINT/SIGTERM 終了検出 (Unix only、Phase 1.14 で Esc 検出削除)
	// Windows では Install() が no-op のため Triggered() は常に false。
	// 終了はウィンドウ X ボタン (GLFW close callback) または Tweaks Quit ボタン。
	if killswitch.Triggered() {
		return ebiten.Termination
	}

	// ─── Phase 4.4: FPS tracking for auto-fallback ────────────────────
	g.fpsFrameCount++
	now := time.Now()
	elapsed := now.Sub(g.fpsLastCheck).Seconds()
	if elapsed >= 1.0 {
		g.fpsCurrentAvg = float64(g.fpsFrameCount) / elapsed
		g.fpsFrameCount = 0
		g.fpsLastCheck = now

		// FPS が 24 を下回ったら morph を一時無効化（配信品質保護）
		// 30 を超えたら再有効化（滞后防止のヒステリシス）
		if g.tweaks.MorphEnabled {
			if g.fpsCurrentAvg < 24.0 {
				g.morphAutoDisable = true
			} else if g.fpsCurrentAvg > 30.0 {
				g.morphAutoDisable = false
			}
		} else {
			g.morphAutoDisable = false
		}
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
	sheetName, sheetIdx := g.sheetForState()

	// Phase 4.0: α ブレンド遷移中は旧セルと新セルを重ねて描画。
	// 遷移中でない場合は従来通り 1 枚描画。
	// Phase 4.1: DrawImage から DrawTriangles (メッシュレンダリング) に切替。
	// Phase 4.2: depth map があるセルは elastic morph を適用。
	// Phase 4.4: morph 不要な場合は depth map 読み込みをスキップ。

	// Phase 4.2: elastic morph target 計算
	// Phase 4.4: morph が完全に無効なら elastic 更新自体をスキップ（EMA 計算省力）
	morphPossible := g.tweaks.MorphEnabled && !g.morphAutoDisable && g.tweaks.MorphStrength > 0

	targetElX := 0.0
	targetElY := 0.0
	if morphPossible && g.mouse != nil {
		mx, my := g.mouse.Current()
		targetElX = mx * float64(g.width) * 0.004
		targetElY = my * float64(g.height) * 0.004
	}
	if morphPossible {
		render.UpdateMorphElastic(&g.morphElastic, targetElX, targetElY)
	}

	// Phase 4.4: depth map 読み込みは morph が有効なときだけ。
	// morph=false の場合、drawMeshWithAlpha は flat mesh パスしか取らないため、
	// depth load + RWMutex lock + string key lookup を完全にスキップできる。
	var fromDepthMap, toDepthMap *image.Gray
	var fromHasDepth, toHasDepth bool
	var fromDepthPath, toDepthPath string

	if g.transEnabled && g.trans.active {
		// 遷移元 (フェードアウト)
		fromImg, fromOk := g.atlas.Get(g.trans.fromSheet, g.trans.fromRow, g.trans.fromCol)
		// 遷移先 (フェードイン)
		toImg, toOk := g.atlas.Get(sheetIdx, row, col)

		if morphPossible {
			// 旧セル / 新セルは別セルの可能性があるので、depth map も個別に読む。
			fromDepthPath = ""
			if fromSheetName := g.atlas.SheetName(g.trans.fromSheet); fromSheetName != "" {
				fromDepthPath = g.atlas.DepthMapPath(fromSheetName, g.trans.fromRow, g.trans.fromCol)
			}
			fromDepthMap, fromHasDepth = render.LoadDepthMap(fromDepthPath)

			toDepthPath = g.atlas.DepthMapPath(sheetName, row, col)
			toDepthMap, toHasDepth = render.LoadDepthMap(toDepthPath)
		}

		if fromOk && fromImg != nil {
			g.drawMeshWithAlpha(screen, fromImg, 1.0-g.trans.progress, fromDepthMap, fromHasDepth, fromDepthPath)
		}
		if toOk && toImg != nil {
			g.drawMeshWithAlpha(screen, toImg, g.trans.progress, toDepthMap, toHasDepth, toDepthPath)
		}
	} else {
		// 従来通り: 単一画像を 100% alpha で描画
		img, ok := g.atlas.Get(sheetIdx, row, col)
		if !ok || img == nil {
			return
		}

		depthPath := ""
		if morphPossible {
			depthPath = g.atlas.DepthMapPath(sheetName, row, col)
			toDepthMap, toHasDepth = render.LoadDepthMap(depthPath)
		}

		g.drawMeshWithAlpha(screen, img, 1.0, toDepthMap, toHasDepth, depthPath)
	}

	if !g.uiHidden && g.tweaks.PanelVisible {
		g.panel.Draw(screen)
	}
}

// drawMeshWithAlpha は画像をメッシュ経由で指定 alpha で screen に描画する。
// Phase 4.1: DrawTriangles ベースの描画パス。
// Phase 4.2: depth map がある場合は elastic morph を適用したメッシュを生成。
// Phase 4.3: MorphEnabled=false または MorphStrength=0 なら flat mesh fallback。
// Phase 4.4: morphed mesh キャッシュ + FPS 自動 fallback。
func (g *Game) drawMeshWithAlpha(screen, img *ebiten.Image, alpha float64, depthMap *image.Gray, hasDepth bool, depthPath string) {
	if img == nil {
		return
	}
	iw, ih := img.Bounds().Dx(), img.Bounds().Dy()

	// Phase 4.4: MorphEnabled=false / MorphStrength=0 / FPS fallback → flat mesh 強制
	morphOk := g.tweaks.MorphEnabled && !g.morphAutoDisable && g.tweaks.MorphStrength > 0 && hasDepth && depthMap != nil

	if morphOk {
		mesh := g.getMorphedMesh(iw, ih, depthMap, depthPath, alpha)
		// Phase 4.4: morphed mesh cache は形状再利用が目的で、alpha は draw ごとに変わる。
		// 遷移中は old/new セルで異なる alpha を使うため、cached mesh を取得した後に
		// 毎回明示的に上書きする。これをしないと前フレームの alpha が残留する。
		// SAFETY: SetAlpha は cached mesh をその場で変更する。Ebitengine の Draw() は
		// 単一 goroutine 前提なので現在は安全。将来 parallel rendering を入れるなら
		// per-draw copy へ切り替える必要がある。
		mesh.SetAlpha(float32(alpha))
		render.DrawMesh(screen, img, mesh)
	} else {
		// Phase 4.1 fallback: フラットメッシュ
		mesh := g.getMeshForImage(iw, ih)
		render.DrawMeshWithAlpha(screen, img, mesh, float32(alpha))
	}
}

// getMorphedMesh は elastic 変位を量子化したキーで morphed mesh をキャッシュから取得する。
// キャッシュミスの場合だけ GenerateMorphedMesh を呼び出し、結果を保存する。
//
// 量子化: elastic 変位を 0.5px 刻みに丸めることで、連続フレーム間の
// 微小変化による再生成を防ぐ。0.5px 未満の差は視認できない。
func (g *Game) getMorphedMesh(imgW, imgH int, depthMap *image.Gray, depthPath string, alpha float64) *render.MeshGrid {
	// elastic 変位を 0.5px 刻みに量子化
	qElX := math.Round(g.morphElastic.ElX*2.0) / 2.0
	qElY := math.Round(g.morphElastic.ElY*2.0) / 2.0

	key := fmt.Sprintf("%d×%d@%d×%d#%.1f×%.1f#%.1f#%s",
		imgW, imgH, g.width, g.height,
		qElX, qElY, g.tweaks.MorphStrength, depthPath)

	g.morphMeshCacheMu.RLock()
	mesh, ok := g.morphMeshCache[key]
	g.morphMeshCacheMu.RUnlock()
	if ok {
		return mesh
	}

	// キャッシュミス → 新規生成
	params := render.MorphParams{
		DepthMap:  depthMap,
		ElX:      g.morphElastic.ElX,
		ElY:      g.morphElastic.ElY,
		Alpha:    1.0,
		Strength: g.tweaks.MorphStrength,
	}
	mesh = render.GenerateMorphedMesh(
		float64(imgW), float64(imgH),
		float64(g.width), float64(g.height),
		params,
	)

	// キャッシュサイズ制限: 256 エントリを超えたら全消去（LRU は YAGNI）。
	// depthPath をキーに含めるため、32 だと通常のマウス移動だけでもすぐ溢れやすい。
	g.morphMeshCacheMu.Lock()
	if len(g.morphMeshCache) > 256 {
		g.morphMeshCache = make(map[string]*render.MeshGrid)
	}
	g.morphMeshCache[key] = mesh
	g.morphMeshCacheMu.Unlock()

	return mesh
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

// sheetForState は現在の (eyesState, mouthState) に対応する sheet name と index を返す。
// character.Config.SheetFor (元 character-config.js の sheets マッピング) に委譲。
func (g *Game) sheetForState() (string, int) {
	return g.atlas.SheetFor(g.eyesClosed, g.mouthState)
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

// getMeshForImage は画像サイズと現在のウィンドウサイズに対応するフラットメッシュを
// キャッシュから取得する。キャッシュにない場合は新規生成して保存する。
//
// Phase 4.1: 頂点座標は screenW/screenH に依存するため、画像サイズだけをキーにすると
// ウィンドウ resize 後に古い座標の mesh を再利用してしまう。key には現在の
// g.width/g.height も含める。
func (g *Game) getMeshForImage(imgW, imgH int) *render.MeshGrid {
	key := fmt.Sprintf("%d×%d@%d×%d", imgW, imgH, g.width, g.height)

	g.meshCacheMu.RLock()
	mesh, ok := g.meshCache[key]
	g.meshCacheMu.RUnlock()
	if ok {
		return mesh
	}

	// キャッシュミス → 新規生成
	mesh = render.GenerateFlatMesh(
		float64(imgW), float64(imgH),
		float64(g.width), float64(g.height),
		1.0, // alpha は DrawMeshWithAlpha で後から設定
	)

	g.meshCacheMu.Lock()
	g.meshCache[key] = mesh
	g.meshCacheMu.Unlock()

	return mesh
}
