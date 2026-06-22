//go:build camera

// Package camera の Phase 2.5 supervisor loop (L3)。
//
// 責務 (docs/PHASE2.md Section 2.5 + Section 1.1 配信中可用性方針):
//
//   - L1 CameraTracker (Phase 2.2) と L2 MPClient (Phase 2.3) の lifecycle 管理
//   - 60Hz supervisor loop で mpclient.Latest() を呼び、顔検出状態を判定
//   - mouse ↔ camera 排他制御 (1秒タイムアウトで mouse mode フォールバック)
//   - L1/L2 の IsRunning() == false 検知 → exponential backoff で自動再起動
//   - supervisor loop 自身の panic 吸収 (defer recover)
//
// 3 層構成:
//
//	┌─────────────────────────────────────────────────────────┐
//	│ L3 Supervisor (本ファイル、Phase 2.5)                  │
//	│   ├─ 60Hz loop: face detection 判定 → mode 切替          │
//	│   ├─ tracker.IsRunning() 監視 → 自動再起動              │
//	│   └─ mpclient.IsRunning() 監視 → 自動再起動             │
//	└─────────────────────────────────────────────────────────┘
//	      ↓ 管理                ↓ 管理
//	┌─────────────────┐    ┌────────────────────┐
//	│ L1 CameraTracker│    │ L2 MPClient        │
//	│ (Phase 2.2)     │    │ (Phase 2.3)        │
//	│ defer recover   │    │ defer recover      │
//	└─────────────────┘    └────────────────────┘
//
// スコープ外 (Phase 2.7 で実装予定):
//   - mp_server.py サブプロセス spawn・監視・再起動
//   - 5 回連続失敗時の Tweaks "Camera Down" 表示
//
// ビルドタグ: `//go:build camera` でガード。Phase 1 ビルドには影響しない。
package camera

import (
	"context"
	"errors"
	"log"
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

// faceDetectionTimeoutSec は「最後の顔検出成功から現在時刻までの猶予秒」。
//
// 1.0 秒 (= 60Hz で約 60 フレーム) を超えたら mouse mode にフォールバックする。
// docs/PHASE2.md §1.1 配信中可用性方針と整合。mapper.go の FaceDetected とは
// 独立に supervisor 側で再評価 (supervisor loop は複数フレームを跨ぐ判定が必要なため)。
//
// 注: mapper.go の faceDetectionTimeoutSec (1.0) と同値。Phase 2.5 では重複だが、
// supervisor は mapper より上位の layer なので独自定数で持つ (Phase 2.6 で統合予定)。
const faceDetectionTimeoutSec = 1.0

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

// Supervisor は L1 (CameraTracker) と L2 (MPClient) の lifecycle +
// mouse/camera 排他制御を管理する L3 supervisor loop。
//
// 設計保証 (Phase 2.5 + 配信中可用性方針 Section 1.1):
//
//  1. クラッシュ安全 — supervisor loop の panic は defer recover で吸収、
//     メイン GoTuber プロセスには影響しない。
//  2. 自動再起動 — L1/L2 が IsRunning() == false になったら exponential backoff で
//     自動再起動 (1s → 2s → 4s → 8s → 16s → 30s 上限、3 回成功でリセット)。
//  3. 排他制御 — 顔検出 1 秒タイムアウトで mouse mode にフォールバック、
//     顔検出復帰で camera mode に自動切替。
//  4. 冪等 Start/Stop — 何ほど呼んでも安全。
//
// YAGNI: tracker / mpclient が nil でも supervisor は lifecycle 管理できる設計
// (supervisor 単体テスト容易性、libzmq 不在環境でもテスト可能)。
//
// 所有権: Start 成功時にのみ loopCtx / cancel の所有権が supervisor へ移る。
// error path は cleanupCtx フラグで defer 経由一括解放
// (Phase 1.14.1 audio capture.go と同パターン、capture.go / mpclient.go と厳密一致)。
type Supervisor struct {
	// Dependencies (immutable after NewSupervisor)
	tracker       *CameraTracker  // Phase 2.2、L1
	mpclient      *MPClient       // Phase 2.3、L2
	mouseFollower *mouse.Follower // Phase 1.12、camera mode → mouse mode 復帰時の参照。
	//                              現状 (Phase 2.5) は保持のみ、Phase 2.6+ で
	//                              pause/active 制御に使う予定 (mouseFollower.Pause() 等)。

	// 内部状態 (mu で保護)
	mu           sync.Mutex
	mode         CameraMode // 1秒タイマー判定後に更新
	lastDetected float64    // Unix 秒、最後の顔検出成功時刻
	faceDetected bool       // 顔検出状態 (1秒タイマーで判定)

	// 起動・終了制御
	cancel context.CancelFunc
	wg     sync.WaitGroup

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
	cameraFps    atomic.Int64           // CameraTracker.SentCount() の snapshot (debug 用)
	detectionFps atomic.Int64           // MPClient.RecvCount() の snapshot (debug 用)
}

// NewSupervisor は L3 supervisor を生成する (L1/L2 は起動しない、Start まで遅延)。
//
// 引数:
//
//	tracker  — L1 CameraTracker (Phase 2.2)、nil 可 (YAGNI: supervisor 単体テスト容易性)
//	mpclient — L2 MPClient (Phase 2.3)、nil 可
//	mouse    — Phase 1.12 MouseFollower (camera → mouse 復帰時の参照、現状は保持のみ)
//
// 戻り値: *Supervisor。stateObserver は即時初期化される。
func NewSupervisor(tracker *CameraTracker, mpclient *MPClient, mouse *mouse.Follower) *Supervisor {
	return &Supervisor{
		tracker:       tracker,
		mpclient:      mpclient,
		mouseFollower: mouse,
		// mode はゼロ値 (CameraModeMouse) で OK、supervisorState.mode と同期される。
		stateObserver: &supervisorState{},
	}
}

// Start は L1/L2 を起動し supervisor loop を別 goroutine で開始する。
//
// フロー:
//
//  1. mutex 内で再チェック (冪等: 既に running なら no-op、Phase 2.2 capture.go と同パターン)
//  2. L1 CameraTracker.Start (失敗時は error path で cleanupCtx 経由解放)
//  3. L2 MPClient.ReceiveLoop を goroutine で起動 (NewMPClient は NewSupervisor 時点で bind 済)
//  4. supervisor loop を goroutine で起動
//
// 冪等: 既に running なら no-op (race-free、mutex 下で re-check)。
//
// エラー時: defer クリーンアップで部分確保リソースを全解放 (cleanupCtx パターン)。
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
	cleanupCtx := true
	defer func() {
		if !cleanupCtx {
			return
		}
		// error path: 起動途中のリソースは goroutine 起動前なので cancel のみで十分。
		cancel()
	}()

	// L1 CameraTracker 起動 (nil なら skip、YAGNI: supervisor 単体テスト用)
	if s.tracker != nil {
		if err := s.tracker.Start(loopCtx); err != nil {
			s.setLastError("tracker.Start: " + err.Error())
			return err
		}
		log.Printf("camera: supervisor: tracker started")
	}

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

// Stop は supervisor loop を停止し、L1/L2 を graceful shutdown する。
//
// フロー:
//
//  1. mutex 内で running をチェック (冪等: 二重 Stop 安全)
//  2. cancel() で loopCtx 終了信号
//  3. wg.Wait() で supervisor loop + MPClient.ReceiveLoop の終了を待つ
//  4. L1 CameraTracker.Close() で webcam/zmq を解放
//
// 任意の goroutine から複数回呼んで OK (cancel ガード + wg.Wait 冪等)。
// Start 前 / Start 失敗後でも安全 (cancel == nil ガード、wg.Wait 即 return)。
func (s *Supervisor) Stop() error {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.mu.Unlock()

	// supervisor loop + MPClient.ReceiveLoop の終了を待つ
	s.wg.Wait()

	// L1 CameraTracker.Close (nil ガード、tracker.Close() 自体が nil-safe)
	if s.tracker != nil {
		if err := s.tracker.Close(); err != nil {
			log.Printf("camera: supervisor: tracker.Close error (ignored): %v", err)
		}
	}
	// MPClient は ReceiveLoop 終了時に releaseResources() で sub/zmqCtx を解放済み。
	// ここでは nil ガード + Close() を呼ぶが、ReceiveLoop が既に終了している場合は
	// 即 return (mpclient.Close の cancel ガードで冪等)。
	if s.mpclient != nil {
		if err := s.mpclient.Close(); err != nil {
			log.Printf("camera: supervisor: mpclient.Close error (ignored): %v", err)
		}
	}

	s.stateObserver.running.Store(false)
	log.Printf("camera: supervisor stopped")
	return nil
}

// supervisorLoop は 60Hz で mpclient.Latest() を呼び、顔検出判定 → mouse/camera 切替。
//
// フロー (各 tick):
//
//  1. mpclient.Latest() で最新 detection 取得 (nil mpclient なら常に faceDetected=false)
//  2. faceDetected 判定 (1秒タイムアウト: now - lastDetected < 1.0)
//  3. mode 切替: faceDetected && mode == Mouse → switchToCameraLocked
//  4. mode 切替: !faceDetected && mode == Camera → switchToMouseLocked
//  5. tracker/mpclient IsRunning() 監視 → 失敗時 exponential backoff で再起動
//
// 終了条件: loopCtx.Done() または panic (defer recover で吸収、goroutine graceful exit)。
//
// defer は LIFO 順で: recover → stateObserver.running.Store(false) → wg.Done。
// この順序で panic 時もメインプロセス無影響 + 状態観測整合。
func (s *Supervisor) supervisorLoop(ctx context.Context) {
	defer s.wg.Done()
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
	trackerBackoff := time.Duration(0)
	trackerFailCount := 0
	trackerSuccessCount := 0
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

		// 2. L1/L2 生存監視 + 再起動 (exponential backoff)
		if s.tracker != nil {
			trackerBackoff, trackerFailCount, trackerSuccessCount = s.monitorAndRestart(
				ctx, "tracker", s.tracker.IsRunning, s.startTracker,
				trackerBackoff, trackerFailCount, trackerSuccessCount,
			)
		}
		if s.mpclient != nil {
			mpclientBackoff, mpclientFailCount, mpclientSuccessCount = s.monitorAndRestart(
				ctx, "mpclient", s.mpclient.IsRunning, s.startMPClient,
				mpclientBackoff, mpclientFailCount, mpclientSuccessCount,
			)
		}

		// 3. FPS snapshot (debug 用、Tweaks 表示は Phase 2.8)
		if s.tracker != nil {
			s.stateObserver.cameraFps.Store(s.tracker.SentCount())
		}
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

	if detected {
		s.lastDetected = nowUnix
		s.faceDetected = true
	} else {
		// 1秒タイムアウト判定: now - lastDetected < 1.0
		// 注: lastDetected がゼロ値 (一度も顔検出していない) のときは false。
		if s.lastDetected > 0 && nowUnix-s.lastDetected < faceDetectionTimeoutSec {
			s.faceDetected = true
		} else {
			s.faceDetected = false
		}
	}

	// mode 切替判定
	switch {
	case s.faceDetected && s.mode == CameraModeMouse:
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
func (s *Supervisor) tickCell(dr DetectionResult, ok bool) {
	if !ok || !dr.FaceDetected {
		s.cellPtr.Store(&cellState{ok: false})
		return
	}

	row := PitchToRow(dr.Pitch)
	col := YawToCol(dr.Yaw)
	eyesClosed := EARToBlink(dr.EarLeft, dr.EarRight) == BlinkClosed
	s.cellPtr.Store(&cellState{row: row, col: col, eyesClosed: eyesClosed, ok: true})
}

// switchToCameraLocked は mouse mode → camera mode 切替 (mu 保護下前提)。
//
// 現状 (Phase 2.5): mode 切り替えと stateObserver 同期のみ。
// mouse follower の pause / camera mapper の active は Phase 2.6+ で実装。
func (s *Supervisor) switchToCameraLocked() {
	s.mode = CameraModeCamera
	s.stateObserver.mode.Store(int32(CameraModeCamera))
	log.Printf("camera: supervisor: mode → Camera (face detected)")
}

// switchToMouseLocked は camera mode → mouse mode 切替 (mu 保護下前提)。
//
// 現状 (Phase 2.5): mode 切り替えと stateObserver 同期のみ。
// mouse follower の active / camera mapper の pause は Phase 2.6+ で実装。
func (s *Supervisor) switchToMouseLocked() {
	s.mode = CameraModeMouse
	s.stateObserver.mode.Store(int32(CameraModeMouse))
	log.Printf("camera: supervisor: mode → Mouse (face lost or timeout)")
}

// monitorAndRestart は L1/L2 の生存を監視し、死亡時に exponential backoff で再起動。
//
// 戻り値: (新しい backoff duration, 新しい fail count, 新しい success count)。
// supervisor loop の 60Hz で毎 tick 呼ばれるので、isAlive callback は lock-free 必須
// (CameraTracker.IsRunning / MPClient.IsRunning は atomic.Bool.Load)。
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

// startTracker は L1 CameraTracker を再起動する (古い tracker の Close → 新規 Start)。
//
// YAGNI: 同一インスタンスの Start は mutex 競合の可能性があるので、Close → 作り直し推奨
// (Phase 2.2 capture.go の Close → NewCameraTracker → Start パターン)。
//
// ただし Phase 2.5 では L1 を supervisor 内で再生成せず、Start のみで再試行する。
// 完全な再生成は Phase 2.7 (port bind 競合回避込み) で実装予定。
func (s *Supervisor) startTracker(ctx context.Context) error {
	if s.tracker == nil {
		return errors.New("tracker is nil")
	}
	return s.tracker.Start(ctx)
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

// setLastError は stateObserver.lastError にエラーメッセージを保存する。
//
// atomic.Pointer[string] は Go 1.19+ の API。エラーメッセージは string として
// immutable に保存される (コピーコスト削減のため unsafe.Pointer 経由)。
func (s *Supervisor) setLastError(msg string) {
	s.stateObserver.lastError.Store(&msg)
	log.Printf("camera: supervisor error: %s", msg)
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
// 取得して mapper.PitchToRow / YawToCol / EARToBlink で cell + blink を計算し、
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
// Phase 2.5: game.Update から呼ばれる lock-free observer。最新 cell snapshot がない場合は
// false に倒し、閉じ誤判定を避ける。
func (s *Supervisor) EyesClosed() bool {
	state := s.cellPtr.Load()
	if state == nil {
		return false
	}
	return state.eyesClosed
}

// IsRunning は supervisor loop が生存中かどうかを返す (atomic.Bool.Load)。
//
// Phase 2.8 で Tweaks パネルの status 表示に使う予定。
func (s *Supervisor) IsRunning() bool {
	return s.stateObserver.running.Load()
}

// LastError は supervisor 内の直近エラーメッセージを返す (lock-free)。
//
// エラー未発生時は空文字列。stateObserver.lastError (atomic.Pointer[string]) を
// 経由して取得。Tweaks パネルの status 表示 (Phase 2.8) で使う予定。
func (s *Supervisor) LastError() string {
	p := s.stateObserver.lastError.Load()
	if p == nil {
		return ""
	}
	return *p
}

// CameraFps は CameraTracker の累計送信フレーム数を返す (debug 用)。
//
// 瞬間 FPS ではなく累計カウント (SentCount 経由)。Tweaks 表示 (Phase 2.8) で
// 経過時間から FPS を計算する想定。
func (s *Supervisor) CameraFps() int64 {
	return s.stateObserver.cameraFps.Load()
}

// DetectionFps は MPClient の累計受信検出数を返す (debug 用)。
func (s *Supervisor) DetectionFps() int64 {
	return s.stateObserver.detectionFps.Load()
}

// anyToString は recover() が返す any (interface{}) を string に変換する。
//
// fmt.Sprintf("%v", v) は recover() で panic value を文字列化する標準パターン。
// supervisorLoop の defer recover 専用ヘルパー。
func anyToString(v any) string {
	if v == nil {
		return "<nil>"
	}
	if err, ok := v.(error); ok {
		return err.Error()
	}
	if s, ok := v.(string); ok {
		return s
	}
	return "non-string panic value"
}
