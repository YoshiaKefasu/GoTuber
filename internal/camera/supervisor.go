//go:build camera

// Package camera の Phase 2.5 supervisor loop (L3)。
//
// 責務 (docs/PHASE2.md Section 2.5 + Section 1.1 配信中可用性方針):
//
//   - L2 MPClient (Phase 2.3) の lifecycle 管理
//   - 60Hz supervisor loop で mpclient.Latest() を呼び、顔検出状態を判定
//   - mouse ↔ camera 排他制御 (1秒タイムアウトで mouse mode フォールバック)
//   - L2 の IsRunning() == false 検知 → exponential backoff で自動再起動
//   - mp_server.py サブプロセス spawn・監視・再起動 (Phase 2.7)
//   - supervisor loop 自身の panic 吸収 (defer recover)
//
// Phase 2.10: CameraTracker (webcam capture) 依存を除去。
// webcam capture は mp_server.py (Python sidecar) が担当。
// Go 側は mp_server.py の spawn/監視 + mpclient 検出結果受信に集中。
//
// 3 層構成:
//
//	┌─────────────────────────────────────────────────────────┐
//	│ L3 Supervisor (本ファイル、Phase 2.5)                  │
//	│   ├─ 60Hz loop: face detection 判定 → mode 切替          │
//	│   ├─ mpclient.IsRunning() 監視 → 自動再起動             │
//	│   └─ mp_server.py 監視 → 自動再起動                      │
//	└─────────────────────────────────────────────────────────┘
//	                     ↓ 管理
//	┌────────────────────────────────────────────┐
//	│ L2 MPClient                                │
//	│ (Phase 2.3)                                │
//	│ defer recover                              │
//	└────────────────────────────────────────────┘
//
// Phase 2.7:
//   - BlinkFilter を tickCell / EyesClosed に統合
//   - mp_server.py サブプロセス spawn・監視・再起動
//
// スコープ外 (Phase 2.8 で実装予定):
//   - 5 回連続失敗時の Tweaks "Camera Down" 表示 UI
//
// ビルドタグ: `//go:build camera` でガード。Phase 1 ビルドには影響しない。
package camera

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/YoshiaKefasu/GoTuber/internal/mouse"
)

// CameraMode は mouse/camera 排他制御の状態。
//
// 整数ベースで `switch` 文に直接渡せるよう明示的な整数値を採番。
// デフォルト (ゼロ値) は CameraModeMouse = 0。Phase 1 ビルドやカメラ無効時に
// 安全側 (= mouse follow) に倒れる設計。
type CameraMode int

const (
	// CameraModeMouse は mouse follow モード (Phase 1 既定)。
	// カメラ無効 / 顔未検出 / supervisor 未起動時に該当。
	CameraModeMouse CameraMode = 0

	// CameraModeCamera は camera follow モード (Phase 2)。
	// 顔検出が 1 秒以上継続している間、supervisor が切替。
	CameraModeCamera CameraMode = 1
)

// supervisorLoopInterval は supervisor loop の tick 間隔。
//
// 60Hz (16ms) は Ebitengine Update ループ (16.6ms) とほぼ同期。
// mpclient.Latest() の mutex 取得は短時間 (struct snapshot のみ) なので
// 60Hz でも負荷は無視できる (実測 < 0.1% CPU、Phase 2.5 budget)。
const supervisorLoopInterval = 16 * time.Millisecond

// supervisor 側は mapper.FaceDetected と独立に複数フレーム跨ぎ判定が必要なため、
// supervisor の s.lastDetected は float64 で独自に保持する。閾値は mapper.go の
// faceDetectionTimeoutSec (1.0) を import 経由で使う (Phase 2.7 で重複を解消)。

// exponential backoff パラメータ (Section 1.1 配信中可用性方針)。
//
// 再起動失敗時にバックオフ: 1s → 2s → 4s → 8s → 16s → 30s (上限)。
// 連続 3 回成功でカウンタリセット。
//
// 注意: Phase 2.7 で mp_server.py サブプロセス再 spawn にも同定数を使う予定なので
// package-private にせず exported にする案もあるが、Phase 2.5 スコープ外なので保留。
const (
	restartBackoffInitial = 1 * time.Second
	restartBackoffMax     = 30 * time.Second
	restartSuccessReset   = 3 // 連続 3 回成功でカウンタリセット
)

// Phase 2.7: mp_server.py サブプロセス再起動バックオフ。
const (
	mpServerInitialBackoff = 1 * time.Second
	mpServerMaxBackoff     = 30 * time.Second
	mpServerMaxFails       = 5 // 5回失敗で manual restart 要求
)

// faceDetectionWarmupTicks は起動時の Camera mode 切替に必要な顔検出連続 tick 数。
// 60Hz で 15 ticks = ~250ms。起動直後の顔検出揺らぎで Mouse↔Camera が
// 連続切替するのを防ぐ (Phase 2.10.6)。
const faceDetectionWarmupTicks = 15

// modeLogCooldown は Mouse/Camera 切替ログの最小出力間隔。
// 実際の mode 切替は維持しつつ、PowerShell を同一系統ログで埋めないための最小限の間引き。
const modeLogCooldown = 12 * time.Second

const errMPServerMaxFails = "mp_server.py 5回連続失敗、手動再起動必要"

// Supervisor は L2 (MPClient) の lifecycle +
// mouse/camera 排他制御を管理する L3 supervisor loop。
//
// 設計保証 (Phase 2.5 + 配信中可用性方針 Section 1.1):
//
//  1. クラッシュ安全 — supervisor loop の panic は defer recover で吸収、
//     メイン GoTuber プロセスには影響しない。
//  2. 自動再起動 — L2 が IsRunning() == false になったら exponential backoff で
//     自動再起動 (1s → 2s → 4s → 8s → 16s → 30s 上限、3 回成功でリセット)。
//  3. 排他制御 — 顔検出 1 秒タイムアウトで mouse mode にフォールバック、
//     顔検出復帰で camera mode に自動切替。
//  4. 冪等 Start/Stop — 何ほど呼んでも安全。
//
// Phase 2.10: CameraTracker (webcam capture) 依存を除去。
// webcam capture は mp_server.py (Python sidecar) が担当。
// Go 側は mp_server.py の spawn/監視 + mpclient 検出結果受信に集中。
//
// YAGNI: mpclient が nil でも supervisor は lifecycle 管理できる設計
// (supervisor 単体テスト容易性、libzmq 不在環境でもテスト可能)。
//
// 所有権: Start 成功時にのみ loopCtx / cancel の所有権が supervisor へ移る。
// error path は cleanupCtx フラグで defer 経由一括解放
// (Phase 1.14.1 audio capture.go と同パターン、mpclient.go と厳密一致)。
type Supervisor struct {
	// Dependencies (immutable after NewSupervisor)
	mpclient      *MPClient       // Phase 2.3、L2
	mouseFollower *mouse.Follower // Phase 1.12、camera mode → mouse mode 復帰時の参照。
	//                              現状 (Phase 2.5) は保持のみ、Phase 2.6+ で
	//                              pause/active 制御に使う予定 (mouseFollower.Pause() 等)。

	// 内部状態 (mu で保護)
	mu           sync.Mutex
	mode         CameraMode // 1秒タイマー判定後に更新
	lastDetected float64    // Unix 秒、最後の顔検出成功時刻
	faceDetected bool       // 顔検出状態 (1秒タイマーで判定)
	faceLostTicks   int  // Phase 2.10.5: 顔未検出連続 tick 数 (grace 用)
	faceDetectedTicks int // 顔検出連続 tick 数 (startup warm-up 用)
	warmupDone      bool // 最初の Camera mode 入力が完了したか (true 以降は gate 不要)

	// 起動・終了制御
	loopCtx context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup

	// Phase 2.6: EAR ヒステリシスフィルタ (Phase 2.7 で supervisor 統合)。
	blinkFilter *BlinkFilter

	// Phase 2.10.4: yaw/pitch smoothing (EMA)。
	// raw 値をそのまま 5x5 セルに通すと微小ノイズでセルが飛び痙攣するため、
	// 指数移動平均 (Exponential Moving Average) で低速追従させる。
	// α = 0.25 (前回値 75% + 新規値 25%) で 60Hz 換算 ~0.3 秒の時定数。
	smoothedYaw   float64
	smoothedPitch float64
	smoothingInit bool // 最初の 1 フレームは raw 値をそのまま代入 (EMA 初期化)
	cellLogCount  int  // Phase 2.10.5: debug log 用カウンタ (60Hz で 1 秒ごと出力)
	lastModeLogAt time.Time

	// Phase 2.7: mp_server.py サブプロセス管理。
	mpServerCmd        *exec.Cmd
	mpServerPath       string
	mpServerDone       chan error
	mpServerEnabled    atomic.Bool
	mpServerRetry      bool
	mpServerFails      int
	mpServerBackoff    time.Duration
	mpServerStableTicks int // 起動後連続 stable tick 数 (fail count リセット用)
	mpSetupRan          bool // Phase 2.10.2: setup script を 1 回だけ実行するガード

	// 観測 (atomic observer、Phase 1.14 規約)
	stateObserver *supervisorState
	cellPtr       atomic.Pointer[cellState]
}

// cellState は camera mode 時に game loop から読む atlas cell snapshot。
//
// Phase 2.5: supervisor loop が 60Hz で mpclient.Latest() を mapper に通し、
// atomic.Pointer で最新値を公開する。game.Draw / Update は mutex なしで読む。
type cellState struct {
	row        int
	col        int
	eyesClosed bool
	ok         bool
}

// supervisorState は L3 supervisor の状態観測 (atomic、外部公開用)。
//
// Supervisor struct 本体の mu は内部状態保護用、supervisorState の atomic は
// lock-free 観測用 (Phase 1.14 audio capture.go の State observer と同パターン)。
//
// running / mode / lastError は Supervisor.mode と冗長だが、atomic 観測専用に
// 分離することで game.Update 等の単一フレーム hot path から mutex なしで読める。
type supervisorState struct {
	running      atomic.Bool            // supervisor loop が生存中
	mode         atomic.Int32           // CameraMode (0/1)
	lastError    atomic.Pointer[string] // 直近エラーメッセージ (Tweaks 表示用、Phase 2.8)
	detectionFps atomic.Int64           // MPClient.RecvCount() の snapshot (debug 用)
}

// NewSupervisor は L3 supervisor を生成する (L2 は起動しない、Start まで遅延)。
//
// 引数:
//
//	mpclient — L2 MPClient (Phase 2.3)、nil 可
//	mouse    — Phase 1.12 MouseFollower (camera → mouse 復帰時の参照、現状は保持のみ)
//
// 戻り値: *Supervisor。stateObserver は即時初期化される。
//
// Phase 2.10: tracker パラメータを削除。webcam capture は mp_server.py が担当。
func NewSupervisor(mpclient *MPClient, mouse *mouse.Follower) *Supervisor {
	return &Supervisor{
		mpclient:      mpclient,
		mouseFollower: mouse,
		blinkFilter:   NewBlinkFilter(),
		// mode はゼロ値 (CameraModeMouse) で OK、supervisorState.mode と同期される。
		stateObserver: &supervisorState{},
	}
}

// Start は L2 を起動し supervisor loop を別 goroutine で開始する。
//
// フロー:
//
//  1. mutex 内で再チェック (冪等: 既に running なら no-op、Phase 2.2 capture.go と同パターン)
//  2. L2 MPClient.ReceiveLoop を goroutine で起動 (NewMPClient は NewSupervisor 時点で bind 済)
//  3. supervisor loop を goroutine で起動
//
// 冪等: 既に running なら no-op (race-free、mutex 下で re-check)。
//
// エラー時: defer クリーンアップで部分確保リソースを全解放 (cleanupCtx パターン)。
//
// Phase 2.10: CameraTracker 起動を削除。webcam capture は mp_server.py が担当。
func (s *Supervisor) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	// mutex 内で再チェック (Phase 2.2 review S-1 修正と同パターン: 並行 Start で
	// loopCtx/cancel の取り合いと goroutine リークを防止)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stateObserver.running.Load() {
		return nil
	}

	loopCtx, cancel := context.WithCancel(ctx)
	s.loopCtx = loopCtx
	if s.blinkFilter == nil {
		s.blinkFilter = NewBlinkFilter()
	} else {
		s.blinkFilter.Reset()
	}
	cleanupCtx := true
	defer func() {
		if !cleanupCtx {
			return
		}
		// error path: 起動途中のリソースは goroutine 起動前なので cancel のみで十分。
		cancel()
	}()

	// L2 MPClient 起動 (nil なら skip、NewMPClient は NewSupervisor 時点で bind 済)
	if s.mpclient != nil {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.mpclient.ReceiveLoop(loopCtx)
		}()
		log.Printf("camera: supervisor: MPClient ReceiveLoop started")
	}

	// supervisor loop 起動 (60Hz)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.supervisorLoop(loopCtx)
	}()
	s.cancel = cancel
	cleanupCtx = false
	s.stateObserver.running.Store(true)
	log.Printf("camera: supervisor started (mode=%d, interval=%v)", s.mode, supervisorLoopInterval)
	return nil
}

// Stop は supervisor loop を停止し、L2 を graceful shutdown する。
//
// フロー:
//
//  1. mutex 内で running をチェック (冪等: 二重 Stop 安全)
//  2. cancel() で loopCtx 終了信号
//  3. wg.Wait() で supervisor loop + MPClient.ReceiveLoop の終了を待つ
//
// 任意の goroutine から複数回呼んで OK (cancel ガード + wg.Wait 冪等)。
// Start 前 / Start 失敗後でも安全 (cancel == nil ガード、wg.Wait 即 return)。
//
// Phase 2.10: CameraTracker.Close 削除。webcam capture は mp_server.py が担当。
func (s *Supervisor) Stop() error {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.loopCtx = nil
	s.mu.Unlock()

	if err := s.stopMPServer(); err != nil {
		log.Printf("camera: supervisor: mp_server.py stop error (ignored): %v", err)
	}

	// supervisor loop + MPClient.ReceiveLoop の終了を待つ
	s.wg.Wait()

	// MPClient は ReceiveLoop 終了時に releaseResources() で conn/listener を解放済み。
	// ここでは nil ガード + Close() を呼ぶが、ReceiveLoop が既に終了している場合は
	// 即 return (mpclient.Close の cancel ガードで冪等)。
	if s.mpclient != nil {
		if err := s.mpclient.Close(); err != nil {
			log.Printf("camera: supervisor: mpclient.Close error (ignored): %v", err)
		}
	}
	if s.blinkFilter != nil {
		s.blinkFilter.Reset()
	}

	// Phase 2.10.6: startup warm-up 状態をリセット。
	// 次回 Start 時に warm-up gate が再適用されるようにする。
	s.faceDetectedTicks = 0
	s.warmupDone = false
	s.lastModeLogAt = time.Time{}

	s.stateObserver.running.Store(false)
	log.Printf("camera: supervisor stopped")
	return nil
}

// anyToString は recover() が返す any (interface{}) を string に変換する。
//
// supervisorLoop の defer recover 専用ヘルパー。
func anyToString(v any) string {
	if v == nil {
		return ""
	}
	if err, ok := v.(error); ok {
		return err.Error()
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// supervisorLoop は 60Hz で mpclient.Latest() を呼び、顔検出判定 → mouse/camera 切替。
//
// フロー (各 tick):
//
//  1. mpclient.Latest() で最新 detection 取得 (nil mpclient なら常に faceDetected=false)
//  2. faceDetected 判定 (1秒タイムアウト: now - lastDetected < 1.0)
//  3. mode 切替: faceDetected && mode == Mouse → switchToCameraLocked
//  4. mode 切替: !faceDetected && mode == Camera → switchToMouseLocked
//  5. mpclient IsRunning() 監視 → 失敗時 exponential backoff で再起動
//  6. mp_server.py サブプロセス監視 → 失敗時 exponential backoff で再起動
//
// 終了条件: loopCtx.Done() または panic (defer recover で吸収、goroutine graceful exit)。
//
// defer は LIFO 順で: recover → stateObserver.running.Store(false)。
// wg.Done は起動側 goroutine の defer が担当し、panic 時もメインプロセス無影響 + 状態観測整合。
//
// Phase 2.10: CameraTracker 監視を削除。webcam capture は mp_server.py が担当。
func (s *Supervisor) supervisorLoop(ctx context.Context) {
	defer s.stateObserver.running.Store(false)

	defer func() {
		if r := recover(); r != nil {
			s.setLastError("supervisorLoop panic: " + anyToString(r))
			log.Printf("camera: supervisor loop panic recovered (goroutine exiting): %v", r)
		}
	}()

	ticker := time.NewTicker(supervisorLoopInterval)
	defer ticker.Stop()

	// 再起動バックオフ state (supervisor lifetime で保持)
	mpclientBackoff := time.Duration(0)
	mpclientFailCount := 0
	mpclientSuccessCount := 0

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		// 1. Latest detection 取得 + faceDetected 判定 + camera cell snapshot 更新
		var dr DetectionResult
		var ok bool
		if s.mpclient != nil {
			dr, _, ok = s.mpclient.Latest()
		}
		s.tickDetectionSnapshot(dr, ok)
		s.tickCell(dr, ok)

		// 2. L2 生存監視 + 再起動 (exponential backoff)
		if s.mpclient != nil {
			mpclientBackoff, mpclientFailCount, mpclientSuccessCount = s.monitorAndRestart(
				ctx, "mpclient", s.mpclient.IsRunning, s.startMPClient,
				mpclientBackoff, mpclientFailCount, mpclientSuccessCount,
			)
		}
		if err := s.monitorMPServer(); err != nil {
			log.Printf("camera: supervisor: mp_server.py monitor error (ignored): %v", err)
		}

		// 3. FPS snapshot (debug 用、Tweaks 表示は Phase 2.8)
		if s.mpclient != nil {
			s.stateObserver.detectionFps.Store(s.mpclient.RecvCount())
		}
	}
}

// tickDetection はテスト用途。supervisorLoop では tickDetectionSnapshot + tickCell を
// 直接呼ぶ。テストでは supervisor 起動不要で state を直接操作できる。
//
// Phase 2.5.1: 削除も検討したが、後方互換性のため残す (Phase 2.6+ で再評価)。
// mpclient.Latest() を呼び、faceDetected を判定、mode 切替。
//
// mutex 保護下で lastDetected / faceDetected / mode を更新し、
// supervisorState.mode (atomic) も同期する。
func (s *Supervisor) tickDetection() {
	var dr DetectionResult
	var ok bool
	if s.mpclient != nil {
		dr, _, ok = s.mpclient.Latest()
	}
	s.tickDetectionSnapshot(dr, ok)
}

func (s *Supervisor) tickDetectionSnapshot(dr DetectionResult, ok bool) {
	nowUnix := float64(time.Now().UnixNano()) / 1e9

	detected := ok && dr.FaceDetected

	s.mu.Lock()
	defer s.mu.Unlock()

	// Phase 2.10.5: faceLostTicks — 顔未検出連続 tick 数をカウント。
	// 1秒 timeout 内でも、一瞬の検出途切れで即 mouse に飛ばすのを防ぐ grace。
	const faceLostGraceTicks = 5 // 60Hz で 5 ticks = ~83ms の grace

	if detected {
		s.lastDetected = nowUnix
		s.faceDetected = true
		s.faceLostTicks = 0
		// Phase 2.10.6: startup warm-up — 連続検出カウントを増やす。
		s.faceDetectedTicks++
	} else {
		// 1秒タイムアウト判定: now - lastDetected < 1.0
		if s.lastDetected > 0 && nowUnix-s.lastDetected < faceDetectionTimeoutSec {
			s.faceDetected = true
			s.faceLostTicks = 0
		} else {
			s.faceLostTicks++
			// grace 期間中は直前の faceDetected を維持
			if s.faceLostTicks <= faceLostGraceTicks {
				s.faceDetected = true
			} else {
				s.faceDetected = false
			}
		}
		// Phase 2.10.6: 顔未検出 → 連続カウントリセット
		s.faceDetectedTicks = 0
	}

	// Phase 2.10.6: startup warm-up gate。
	// 最初の Camera mode 入力前に限って、顔検出が一定回数連続するまで
	// Mouse→Camera 切替を保留する。これにより起動直後の mode フラッピングを防止。
	// warmupDone=true 以降は gate を通過し、通常の lost-signal フォールバック速度は維持。
	readyForCamera := s.warmupDone || s.faceDetectedTicks >= faceDetectionWarmupTicks

	// mode 切替判定
	switch {
	case s.faceDetected && s.mode == CameraModeMouse && readyForCamera:
		s.switchToCameraLocked()
	case !s.faceDetected && s.mode == CameraModeCamera:
		s.switchToMouseLocked()
	}
}

// tickCell は最新 detection から camera mode 用の atlas cell と瞬き状態を計算する。
//
// Phase 2.5: mpclient.Latest() の結果を mapper.go の純粋関数に通し、atomic snapshot として
// game パッケージに公開する。顔未検出 / 最新値なしの場合は ok=false を保存し、game 側は
// mouse.Cell() にフォールバックする。
//
// Phase 2.10.5: 実機トラッキング安定化。
//   - EMA smoothing α=0.18 (前回値 82% + 新規値 18%) で 60Hz 換算 ~0.45 秒の時定数
//   - deadzone: |yaw| < 3° or |pitch| < 3° → 0 扱い (正面付近の微振動を殺す)
//   - debug log: 1秒ごとに raw/smoothed 値を出力
func (s *Supervisor) tickCell(dr DetectionResult, ok bool) {
	if !ok || !dr.FaceDetected {
		s.cellPtr.Store(&cellState{ok: false})
		return
	}

	// Phase 2.10.5: deadzone — 正面付近の微小ノイズを 0 に丸める。
	// これにより中央セル (row=2, col=2) 付近の痙攣が大幅に軽減される。
	const deadzoneDeg = 3.0
	yaw := dr.Yaw
	pitch := dr.Pitch
	if yaw > -deadzoneDeg && yaw < deadzoneDeg {
		yaw = 0
	}
	if pitch > -deadzoneDeg && pitch < deadzoneDeg {
		pitch = 0
	}

	// Phase 2.10.5: EMA smoothing — α=0.18 で少し強めに平滑化。
	const smoothingAlpha = 0.18

	if !s.smoothingInit {
		s.smoothedYaw = yaw
		s.smoothedPitch = pitch
		s.smoothingInit = true
	} else {
		s.smoothedYaw = s.smoothedYaw*(1-smoothingAlpha) + yaw*smoothingAlpha
		s.smoothedPitch = s.smoothedPitch*(1-smoothingAlpha) + pitch*smoothingAlpha
	}

	row := PitchToRow(s.smoothedPitch)
	col := YawToCol(s.smoothedYaw)
	if s.blinkFilter == nil {
		s.blinkFilter = NewBlinkFilter()
	}
	eyesClosed := s.blinkFilter.Update(dr.EarLeft, dr.EarRight) == BlinkClosed
	s.cellPtr.Store(&cellState{row: row, col: col, eyesClosed: eyesClosed, ok: true})

	// Phase 2.10.5: debug log — 1秒ごとに raw/smoothed/cell を出力。
	// rawYaw は deadzone 適用前、smoothedYaw は EMA 適用後の値。
	s.cellLogCount++
	if s.cellLogCount >= 60 { // 60Hz → 1秒ごと
		s.cellLogCount = 0
		log.Printf(
			"camera: debug cell: raw=(%.1f,%.1f) smooth=(%.1f,%.1f) cell=(r%d,c%d) deadzone=%.0f° alpha=%.2f",
			dr.Yaw, dr.Pitch,
			s.smoothedYaw, s.smoothedPitch,
			row, col,
			deadzoneDeg, smoothingAlpha,
		)
	}
}

// switchToCameraLocked は mouse mode → camera mode 切替 (mu 保護下前提)。
//
// 現状 (Phase 2.5): mode 切り替えと stateObserver 同期のみ。
// mouse follower の pause / camera mapper の active は Phase 2.6+ で実装。
func (s *Supervisor) switchToCameraLocked() {
	s.mode = CameraModeCamera
	s.stateObserver.mode.Store(int32(CameraModeCamera))
	// Phase 2.10.6: 最初の Camera mode 入力完了。以降は warm-up gate 不要。
	s.warmupDone = true
	s.logModeTransitionLocked("camera: supervisor: mode → Camera (face detected)")
}

// switchToMouseLocked は camera mode → mouse mode 切替 (mu 保護下前提)。
//
// 現状 (Phase 2.5): mode 切り替えと stateObserver 同期のみ。
// mouse follower の active / camera mapper の pause は Phase 2.6+ で実装。
func (s *Supervisor) switchToMouseLocked() {
	s.mode = CameraModeMouse
	s.stateObserver.mode.Store(int32(CameraModeMouse))
	if s.blinkFilter != nil {
		s.blinkFilter.Reset()
	}
	// Phase 2.10.4: camera → mouse 切替時に smoothing 状態をリセット。
	// 次回 camera 復帰時に stale な smoothed 値を引き継がないため。
	s.smoothingInit = false
	s.faceLostTicks = 0
	s.cellLogCount = 0
	s.logModeTransitionLocked("camera: supervisor: mode → Mouse (face lost or timeout)")
}

// logModeTransitionLocked は mode 切替ログの連発を抑える。
// 実際の Mouse/Camera 切替は変えず、ログだけ 1 秒に 1 回までへ間引く。
// mu 保護下前提。
func (s *Supervisor) logModeTransitionLocked(line string) {
	now := time.Now()
	if !s.lastModeLogAt.IsZero() && now.Sub(s.lastModeLogAt) < modeLogCooldown {
		return
	}
	log.Print(line)
	s.lastModeLogAt = now
}

// monitorAndRestart は L1/L2 の生存を監視し、死亡時に exponential backoff で再起動。
//
// 戻り値: (新しい backoff duration, 新しい fail count, 新しい success count)。
// supervisor loop の 60Hz で毎 tick 呼ばれるので、isAlive callback は lock-free 必須
// (MPClient.IsRunning は atomic.Bool.Load)。
func (s *Supervisor) monitorAndRestart(
	ctx context.Context,
	name string,
	isAlive func() bool,
	restart func(ctx context.Context) error,
	backoff time.Duration,
	failCount, successCount int,
) (time.Duration, int, int) {
	// backoff 中はスキップ
	if backoff > 0 {
		backoff -= supervisorLoopInterval
		if backoff < 0 {
			backoff = 0
		}
		return backoff, failCount, successCount
	}

	if isAlive() {
		// 生存: successCount をインクリメント、3 連続成功で failCount リセット
		if failCount > 0 {
			successCount++
			if successCount >= restartSuccessReset {
				failCount = 0
				successCount = 0
				log.Printf("camera: supervisor: %s recovered (backoff reset)", name)
			}
		}
		return 0, failCount, successCount
	}

	// 死亡: 再起動試行
	failCount++
	successCount = 0
	log.Printf("camera: supervisor: %s not running, restart attempt #%d", name, failCount)
	if err := restart(ctx); err != nil {
		s.setLastError(name + " restart failed: " + err.Error())
		backoff = nextBackoff(failCount)
		log.Printf("camera: supervisor: %s restart failed (backoff %v): %v", name, backoff, err)
		return backoff, failCount, successCount
	}

	// 再起動成功: バックオフなし (次の tick で alive 判定)
	log.Printf("camera: supervisor: %s restarted successfully", name)
	return 0, failCount, successCount
}

// nextBackoff は exponential backoff の次の待機時間を返す。
//
// 1, 2, 4, 8, 16, 30 (上限) 秒。failCount=1 → 1s、2 → 2s、3 → 4s、...
func nextBackoff(failCount int) time.Duration {
	if failCount <= 0 {
		return 0
	}
	// 2^(failCount-1) 秒、上限 30s
	d := restartBackoffInitial
	for i := 1; i < failCount; i++ {
		d *= 2
		if d >= restartBackoffMax {
			return restartBackoffMax
		}
	}
	if d > restartBackoffMax {
		return restartBackoffMax
	}
	return d
}

// startMPClient は L2 MPClient の ReceiveLoop を再起動する。
//
// MPClient は NewMPClient 時点で bind 済なので、Close → NewMPClient → ReceiveLoop の
// 完全再生成が必要 (port bind 競合回避)。Phase 2.5 では ReceiveLoop 単体の再起動を
// 試みるが、port bind 失敗時は NewMPClient 必要。
func (s *Supervisor) startMPClient(ctx context.Context) error {
	if s.mpclient == nil {
		return errors.New("mpclient is nil")
	}
	// 既に running なら no-op (race-free: ReceiveLoop 自身が re-check する)
	if s.mpclient.IsRunning() {
		return nil
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.mpclient.ReceiveLoop(ctx)
	}()
	return nil
}

// StartMPServer は mp_server.py サブプロセスを起動する公開 wrapper。
//
// Phase 2.7: camera_hook_camera.go から起動 trigger するための薄い wrapper。
// 起動失敗時も GoTuber 本体は継続し、以後 monitorMPServer が backoff 付き再起動を試みる。
func (s *Supervisor) StartMPServer() error {
	return s.startMPServer()
}

// RestartMPServer は mp_server.py サブプロセスを強制再起動する。
//
// Phase 2.8: Tweaks panel の Manual Restart ボタンから呼ばれる公開 API。
// Down 状態 (5回失敗到達) からの復帰、または任意タイミングの手動再起動用。
// 成功時のみ lastError をリセットする。fail count / backoff は手動連打で壊さないため保持する。
func (s *Supervisor) RestartMPServer() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.mpServerCmd != nil && s.mpServerCmd.Process != nil {
		_ = s.mpServerCmd.Process.Kill()
	}
	// Phase 2.8.1: Kill 直後の port 解放 race を避けるため、旧 process の Wait 完了を
	// 最大 1 秒だけ待つ。超過時は配信中操作の体感を優先して次の起動へ進む。
	if s.mpServerDone != nil {
		select {
		case <-s.mpServerDone:
		case <-time.After(1 * time.Second):
		}
	}
	s.mpServerCmd = nil
	s.mpServerDone = nil
	s.mpServerEnabled.Store(false)
	s.mpServerRetry = true

	if err := s.startMPServerLocked(); err != nil {
		s.setLastErrorLocked("mp_server.py manual restart failed: " + err.Error())
		return err
	}
	s.clearLastErrorLocked()
	return nil
}

// startMPServer は mp_server.py サブプロセスを起動する。
//
// Phase 2.7: 冪等。既に起動中なら no-op。Python サイドカーの起動失敗は呼び出し側で
// graceful degradation し、mouse mode で配信継続する。
//
// Phase 2.10.3: ensureVenvAndDeps() は mutex 外で実行。
// 初回セットアップ時の pip install (2-5 分) が Stop() の mutex 取得を妨げないようにする。
func (s *Supervisor) startMPServer() error {
	// Phase 2.10.3: setup 実行は mutex 外。blocking しても graceful shutdown を遅延させない。
	if err := s.ensureVenvAndDeps(); err != nil {
		log.Printf("camera: supervisor: auto-setup skipped: %v", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.mpServerRetry = true
	return s.startMPServerLocked()
}

// startMPServerLocked は mp_server.py を起動する内部メソッド (mutex 保持下)。
func (s *Supervisor) startMPServerLocked() error {
	if s.mpServerCmd != nil && s.mpServerDone != nil {
		select {
		case <-s.mpServerDone:
			// 終了済みなら下で再起動する。
		default:
			return nil
		}
	}
	if s.mpServerEnabled.Load() && s.mpServerCmd != nil {
		return nil
	}

	pythonExe, err := pythonExecutable()
	if err != nil {
		return err
	}
	mpServerPath, err := s.resolveMPServerPath()
	if err != nil {
		return err
	}

	ctx := s.loopCtx
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, pythonExe, mpServerPath)

	// Phase 2.10.1: mp_server.py の stdout/stderr を Go ログに流す。
	// Python 側の import error / camera open failure / クラッシュ原因が
	// PowerShell で直接読めるようにする。
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("mp_server.py stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("mp_server.py stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("mp_server.py start failed: %w", err)
	}

	// stdout/stderr を prefix 付きで Go ログに転送 (goroutine)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			log.Printf("camera: mp_server.py: %s", scanner.Text())
		}
	}()
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			log.Printf("camera: mp_server.py [stderr]: %s", scanner.Text())
		}
	}()

	done := make(chan error, 1)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		done <- cmd.Wait()
	}()

	s.mpServerCmd = cmd
	s.mpServerPath = mpServerPath
	s.mpServerDone = done
	s.mpServerEnabled.Store(true)
	// Phase 2.10.1: fail count / backoff はリセットしない。
	// monitorMPServer() 内で安定稼働 (successCount) でリセットする。
	// 連続即死時に fail count が 1 に戻るバグを防止。
	s.mpServerStableTicks = 0
	log.Printf("camera: supervisor: mp_server.py started (pid=%d, path=%s)", cmd.Process.Pid, mpServerPath)
	return nil
}

// stopMPServer は mp_server.py サブプロセスを graceful shutdown する。
//
// Phase 2.7.2: SIGTERM 相当の Interrupt → 待機 → Kill。Windows は Interrupt が
// 未対応の可能性が高く、配信停止体感を優先して待機を 1 秒に短縮する。
func (s *Supervisor) stopMPServer() error {
	s.mu.Lock()
	cmd := s.mpServerCmd
	done := s.mpServerDone
	s.mpServerCmd = nil
	s.mpServerDone = nil
	s.mpServerEnabled.Store(false)
	s.mpServerRetry = false
	s.mpServerFails = 0
	s.mpServerStableTicks = 0
	s.mu.Unlock()

	if cmd == nil || cmd.Process == nil {
		return nil
	}
	// done != nil チェック: startMPServer 経由で必ず作られるが、Stop 経由や
	// 初期状態では nil の可能性がある。nil チェックで早期 return。
	if done != nil {
		select {
		case <-done:
			return nil
		default:
		}
	}

	if runtime.GOOS != "windows" {
		_ = cmd.Process.Signal(os.Interrupt)
	}
	gracefulTimeout := 5 * time.Second
	if runtime.GOOS == "windows" {
		// Windows: Python SIGTERM handler が機能しないことが多い。
		// cv2.VideoCapture.release 中の 5秒は体感が悪いため 1秒に短縮する。
		gracefulTimeout = 1 * time.Second
	}
	if done != nil {
		select {
		case <-done:
			return nil
		case <-time.After(gracefulTimeout):
		}
	}
	return cmd.Process.Kill()
}

// monitorMPServer は mp_server.py の生存確認と backoff 付き再起動を行う。
//
// Phase 2.7: supervisorLoop から 60Hz で呼ぶ。実 subprocess の起動失敗や終了は
// GoTuber 本体に伝播させず、5回連続失敗で manual restart 要求を lastError に残す。
//
// Phase 2.10.1: fail count リセットは「起動成功」ではなく「安定稼働」時のみ。
// mpServerStableTicks で連続生存 tick 数をカウントし、5秒 (300 ticks) 稼働後に
// fail count / backoff をリセットする。連続即死時に fail count が 1 に戻るバグを防止。
func (s *Supervisor) monitorMPServer() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.mpServerRetry {
		return nil
	}
	if s.mpServerFails >= mpServerMaxFails {
		s.setLastErrorLocked(errMPServerMaxFails)
		return nil
	}

	if s.mpServerDone != nil {
		select {
		case err := <-s.mpServerDone:
			s.mpServerCmd = nil
			s.mpServerDone = nil
			s.mpServerEnabled.Store(false)
			s.mpServerFails++
			s.mpServerStableTicks = 0
			s.mpServerBackoff = nextMPServerBackoff(s.mpServerFails)
			if err != nil {
				s.setLastErrorLocked("mp_server.py exited: " + err.Error())
			}
			log.Printf("camera: supervisor: mp_server.py exited (fail count %d, backoff %v)", s.mpServerFails, s.mpServerBackoff)
		default:
			// Phase 2.10.1: 安定稼働カウント (60Hz = 300 ticks で 5 秒)。
			// mp_server.py が落ちずに生存中なら tick を増やし、
			// 安定期間到達時に fail count / backoff をリセットする。
			s.mpServerStableTicks++
			if s.mpServerStableTicks >= 300 && s.mpServerFails > 0 {
				log.Printf("camera: supervisor: mp_server.py stable for 5s (fail count %d → 0)", s.mpServerFails)
				s.mpServerFails = 0
				s.mpServerBackoff = 0
			}
			return nil
		}
	}

	if s.mpServerBackoff > 0 {
		s.mpServerBackoff -= supervisorLoopInterval
		if s.mpServerBackoff < 0 {
			s.mpServerBackoff = 0
		}
		return nil
	}

	if err := s.startMPServerLocked(); err != nil {
		s.mpServerFails++
		s.mpServerBackoff = nextMPServerBackoff(s.mpServerFails)
		if s.mpServerFails >= mpServerMaxFails {
			s.setLastErrorLocked(errMPServerMaxFails)
		} else {
			s.setLastErrorLocked("mp_server.py start failed: " + err.Error())
		}
		return err
	}
	return nil
}

func (s *Supervisor) resolveMPServerPath() (string, error) {
	if s.mpServerPath != "" {
		return s.mpServerPath, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for _, candidate := range []string{
		filepath.Join(wd, "tools", "mp_server.py"),
		filepath.Join(wd, "..", "..", "tools", "mp_server.py"),
	} {
		abs, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		if _, err := os.Stat(abs); err == nil {
			return abs, nil
		}
	}
	return "", errors.New("tools/mp_server.py not found")
}

func pythonExecutable() (string, error) {
	// Phase 2.10.2: .venv-mp 内の Python を最優先。
	// gotuber-camera.exe からの自動起動時、ユーザーが手動で venv を作っていなくても
	// ensureVenvAndDeps() で自動作成される前提。
	wd, _ := os.Getwd()
	if wd == "" {
		wd, _ = filepath.Abs(".")
	}
	venvCandidates := []string{}
	if runtime.GOOS == "windows" {
		venvCandidates = []string{filepath.Join(wd, ".venv-mp", "Scripts", "python.exe")}
	} else {
		venvCandidates = []string{filepath.Join(wd, ".venv-mp", "bin", "python")}
	}
	for _, venvPython := range venvCandidates {
		if _, err := os.Stat(venvPython); err == nil {
			return venvPython, nil
		}
	}

	// fallback: PATH 上の python
	pathCandidates := []string{"python3", "python"}
	if runtime.GOOS == "windows" {
		pathCandidates = []string{"python.exe", "python3.exe", "python"}
	}
	for _, candidate := range pathCandidates {
		path, err := exec.LookPath(candidate)
		if err == nil {
			return path, nil
		}
	}
	return "", errors.New("python executable not found (install Python 3.9+ or run tools/setup-mp.ps1)")
}

// ensureVenvAndDeps は .venv-mp が無ければ setup script を自動実行する。
//
// Phase 2.10.2: gotuber-camera.exe 起動時に venv / 依存が未セットアップでも
// 自動で前に進むためのフック。mpSetupRan ガードで 1 回きり実行 (無限ループ防止)。
func (s *Supervisor) ensureVenvAndDeps() error {
	if s.mpSetupRan {
		return nil
	}
	s.mpSetupRan = true

	// 既に venv python が存在するなら setup 不要
	if _, err := pythonExecutable(); err == nil {
		wd, _ := os.Getwd()
		venvPython := filepath.Join(wd, ".venv-mp", "Scripts", "python.exe")
		if runtime.GOOS != "windows" {
			venvPython = filepath.Join(wd, ".venv-mp", "bin", "python")
		}
		if _, err := os.Stat(venvPython); err == nil {
			return nil // venv python が存在 → setup 不要
		}
	}

	// setup script を探す
	wd, _ := os.Getwd()
	var setupScript string
	if runtime.GOOS == "windows" {
		setupScript = filepath.Join(wd, "tools", "setup-mp.ps1")
	} else {
		setupScript = filepath.Join(wd, "tools", "setup-mp.sh")
	}
	if _, err := os.Stat(setupScript); err != nil {
		return fmt.Errorf("setup script not found: %s (run manually: tools/setup-mp.ps1)", setupScript)
	}

	log.Printf("camera: supervisor: .venv-mp が見つかりません。自動セットアップを開始します (初回のみ) ...")

	// setup script を実行
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		// PowerShell で実行
		cmd = exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", setupScript)
	} else {
		cmd = exec.Command("bash", setupScript)
	}
	cmd.Dir = wd
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("auto-setup failed: %w (run tools/setup-mp.ps1 manually)", err)
	}

	log.Printf("camera: supervisor: 自動セットアップ完了")
	return nil
}

func nextMPServerBackoff(failCount int) time.Duration {
	if failCount <= 0 {
		return 0
	}
	d := mpServerInitialBackoff
	for i := 1; i < failCount; i++ {
		d *= 2
		if d >= mpServerMaxBackoff {
			return mpServerMaxBackoff
		}
	}
	if d > mpServerMaxBackoff {
		return mpServerMaxBackoff
	}
	return d
}

// setLastError は stateObserver.lastError にエラーメッセージを保存する。
//
// atomic.Pointer[string] は Go 1.19+ の API。エラーメッセージは string として
// immutable に保存される (コピーコスト削減のため unsafe.Pointer 経由)。
func (s *Supervisor) setLastError(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.storeLastError(msg)
}

func (s *Supervisor) setLastErrorLocked(msg string) {
	s.storeLastError(msg)
}

func (s *Supervisor) storeLastError(msg string) {
	if cur := s.stateObserver.lastError.Load(); cur != nil && *cur == msg {
		return
	}
	s.stateObserver.lastError.Store(&msg)
	log.Printf("camera: supervisor error: %s", msg)
}

func (s *Supervisor) clearLastErrorLocked() {
	s.stateObserver.lastError.Store(nil)
}

// --- 状態 observer (lock-free, 任意の goroutine から安全) ---

// Mode は現在の mouse/camera mode を返す (lock-free、atomic.Int32.Load)。
//
// Phase 1 ビルド / カメラ無効 / supervisor 未起動時は CameraModeMouse (0) を返す。
// game.Update の 60Hz ループから呼ばれる hot path 用。
func (s *Supervisor) Mode() CameraMode {
	return CameraMode(s.stateObserver.mode.Load())
}

// CameraCell は camera mode 時の atlas cell (row, col) を返す。
//
// Phase 2.5.1: supervisorLoop の tickCell() が cellPtr に書き込んだ cellState を
// atomic.Pointer で読み出して返す。毎フレーム supervisorLoop が mpclient.Latest() を
// 取得して mapper.PitchToRow / YawToCol と BlinkFilter.Update で cell + blink を計算し、
// 結果を cellPtr に Store する。game.Draw() / Update() は mutex なしで読む。
// 最新 cell がない場合は (2, 2, false) を返し、呼び出し側が fallback を選べるようにする。
//
// game.go の Update から呼ばれる。lock-free (mutex 不使用)。
func (s *Supervisor) CameraCell() (row, col int, ok bool) {
	state := s.cellPtr.Load()
	if state == nil || !state.ok {
		return 2, 2, false
	}
	return state.row, state.col, true
}

// EyesClosed は camera mode 時の EAR ベース瞬き状態を返す。
//
// Phase 2.7: game.Update から呼ばれる lock-free observer。BlinkFilter の現在 state が
// BlinkClosed の場合のみ true を返す。mouse fallback 時は switchToMouseLocked / Stop で Reset する。
func (s *Supervisor) EyesClosed() bool {
	if s.blinkFilter == nil {
		return false
	}
	return s.blinkFilter.State() == BlinkClosed
}

// IsRunning は supervisor loop が生存中かどうかを返す (atomic.Bool.Load)。
//
// Phase 2.8 で Tweaks パネルの status 表示に使う予定。
func (s *Supervisor) IsRunning() bool {
	return s.stateObserver.running.Load()
}

// LastError は supervisor 内の直近エラーメッセージを返す (lock-free)。
//
// エラー未発生時は nil。stateObserver.lastError (atomic.Pointer[string]) を
// 経由して取得。Tweaks パネルの status 表示 (Phase 2.8) で使う予定。
func (s *Supervisor) LastError() *string {
	return s.stateObserver.lastError.Load()
}

// MPServerRunning は mp_server.py プロセスが現在動作中かを返す。
//
// Phase 2.8: Tweaks panel の Camera Status 表示用。新規 observer フィールドは増やさず、
// supervisor が保持するプロセス状態から最小限に算出する。
func (s *Supervisor) MPServerRunning() bool {
	return s.mpServerEnabled.Load()
}

// DetectionFps は MPClient の累計受信検出数を返す (debug 用)。
func (s *Supervisor) DetectionFps() int64 {
	return s.stateObserver.detectionFps.Load()
}
