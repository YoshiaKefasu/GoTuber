//go:build camera

// Package camera の Phase 2.5 supervisor loop (L3) のユニットテスト。
//
// テスト方針 (Phase 1.14 規約):
//   - stdlib のみ使用 (libzmq 不要、テスト実行可能)
//   - mpclient = nil でも supervisor の lifecycle 管理は動作する設計を検証
//   - private メソッド (tickDetection / switchToCameraLocked) は同一パッケージ内なので直接呼べる
//   - supervisor loop の 60Hz tick は短い Sleep で代用 (CI 高速化)
//
// 注: これらのテストは -tags camera でのみ実行される (libzmq 不在環境では go test ./...
// がスキップされる、Phase 1 テストには影響しない)。
//
// Phase 2.10: tracker パラメータ削除に合わせて NewSupervisor 呼び出しを 2 引数化。
package camera

import (
	"strings"
	"testing"
	"time"
)

// TestSupervisor_DefaultState は NewSupervisor 直後の state を検証する。
//
// 起動前:
//   - Mode() == CameraModeMouse (デフォルト)
//   - IsRunning() == false (loop 未起動)
//   - LastError() == nil (エラーなし)
//   - DetectionFps == 0 (カウント未開始)
func TestSupervisor_DefaultState(t *testing.T) {
	s := NewSupervisor(nil, nil)
	if got := s.Mode(); got != CameraModeMouse {
		t.Errorf("Mode() before Start = %d, want %d (CameraModeMouse)", got, CameraModeMouse)
	}
	if s.IsRunning() {
		t.Errorf("IsRunning() before Start = true, want false")
	}
	if s.LastError() != nil {
		t.Errorf("LastError() before Start = %q, want nil", *s.LastError())
	}
	if s.DetectionFps() != 0 {
		t.Errorf("DetectionFps() before Start = %d, want 0", s.DetectionFps())
	}
}

// TestSupervisor_NewSupervisor_DefaultMode は NewSupervisor が mouse mode (ゼロ値) を返すことを確認。
//
// Phase 1 ビルドやカメラ無効時に安全側 (= mouse follow) に倒れる設計の検証。
// stateObserver.mode (atomic) と Supervisor.mode (mu 保護下の内部状態) の両方を確認。
func TestSupervisor_NewSupervisor_DefaultMode(t *testing.T) {
	s := NewSupervisor(nil, nil)
	if got := s.Mode(); got != CameraModeMouse {
		t.Errorf("NewSupervisor default Mode() = %d, want %d", got, CameraModeMouse)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.mode != CameraModeMouse {
		t.Errorf("NewSupervisor default mode field = %d, want %d", s.mode, CameraModeMouse)
	}
}

// TestSupervisor_Stop_NeverStarted_NoPanic は未起動の supervisor に対する Stop が panic なしで
// 動作することを確認 (冪等性 + nil 安全性)。
func TestSupervisor_Stop_NeverStarted_NoPanic(t *testing.T) {
	s := NewSupervisor(nil, nil)
	// Never started, but Stop should not panic
	if err := s.Stop(); err != nil {
		t.Errorf("Stop() before Start = %v, want nil", err)
	}
	// Idempotent: second Stop also OK
	if err := s.Stop(); err != nil {
		t.Errorf("Stop() second call = %v, want nil", err)
	}
	// Third call still OK
	if err := s.Stop(); err != nil {
		t.Errorf("Stop() third call = %v, want nil", err)
	}
}

// TestSupervisor_StartStop_Lifecycle は supervisor loop の Start → 短時間動作 → Stop を確認。
//
// 検証項目:
//   - Start() 成功、IsRunning() == true
//   - 100ms 経過後も IsRunning() == true (loop が安定動作)
//   - Stop() 成功、IsRunning() == false
//   - tracker=nil, mpclient=nil でも OK (YAGNI 設計)
func TestSupervisor_StartStop_Lifecycle(t *testing.T) {
	s := NewSupervisor(nil, nil)
	if err := s.Start(nil); err != nil {
		t.Fatalf("Start() = %v, want nil", err)
	}
	if !s.IsRunning() {
		t.Errorf("IsRunning() after Start = false, want true")
	}
	// Wait 100ms for supervisor loop to tick a few times
	time.Sleep(100 * time.Millisecond)
	if !s.IsRunning() {
		t.Errorf("IsRunning() after 100ms = false, want true")
	}
	if err := s.Stop(); err != nil {
		t.Errorf("Stop() = %v, want nil", err)
	}
	if s.IsRunning() {
		t.Errorf("IsRunning() after Stop = true, want false")
	}
}

// TestSupervisor_Start_Idempotent は二重 Start が panic なしで no-op であることを確認。
//
// 並行 Start や Start → Stop → Start のサイクルで問題が出ないことの確認。
func TestSupervisor_Start_Idempotent(t *testing.T) {
	s := NewSupervisor(nil, nil)
	if err := s.Start(nil); err != nil {
		t.Fatalf("first Start() = %v, want nil", err)
	}
	defer s.Stop()
	// Second Start should not panic and should be no-op
	if err := s.Start(nil); err != nil {
		t.Errorf("second Start() = %v, want nil (idempotent)", err)
	}
	if !s.IsRunning() {
		t.Errorf("IsRunning() after double Start = false, want true")
	}
	// Third Start still OK
	if err := s.Start(nil); err != nil {
		t.Errorf("third Start() = %v, want nil (idempotent)", err)
	}
}

// TestSupervisor_ModeAtomicObserver は Mode() の lock-free 読み出しを確認。
//
// stateObserver.mode (atomic.Int32) を直接 Store して、Mode() 経由で即座に
// 観測できることを検証。これは game.Update の hot path で mutex なしで mode を
// 読む設計の正当性確認。
func TestSupervisor_ModeAtomicObserver(t *testing.T) {
	s := NewSupervisor(nil, nil)

	// Mouse mode (default)
	if got := s.Mode(); got != CameraModeMouse {
		t.Errorf("Mode() default = %d, want %d", got, CameraModeMouse)
	}

	// Switch to Camera via atomic write
	s.stateObserver.mode.Store(int32(CameraModeCamera))
	if got := s.Mode(); got != CameraModeCamera {
		t.Errorf("Mode() after atomic write = %d, want %d", got, CameraModeCamera)
	}

	// Reset to Mouse
	s.stateObserver.mode.Store(int32(CameraModeMouse))
	if got := s.Mode(); got != CameraModeMouse {
		t.Errorf("Mode() after reset = %d, want %d", got, CameraModeMouse)
	}
}

// TestSupervisor_SwitchToCamera_UpdatesMode は tickDetection が mode を mouse → camera に
// 切替えることを確認。
//
// supervisor loop を起動せず、tickDetection を手動で呼ぶことで race condition を排除。
// faceDetected = true + lastDetected = now を強制することで switchToCameraLocked が
// 呼ばれ、Supervisor.mode と stateObserver.mode (Mode()) の両方が更新される。
func TestSupervisor_SwitchToCamera_UpdatesMode(t *testing.T) {
	s := NewSupervisor(nil, nil)
	// 起動しない (supervisor loop と race させない)

	// 初期状態確認
	if got := s.Mode(); got != CameraModeMouse {
		t.Fatalf("initial Mode() = %d, want %d (CameraModeMouse)", got, CameraModeMouse)
	}

	// faceDetected=true + lastDetected=now + warm-up gate 条件充足を強制 (mu 保護下)
	nowUnix := float64(time.Now().UnixNano()) / 1e9
	s.mu.Lock()
	s.faceDetected = true
	s.lastDetected = nowUnix
	s.warmupDone = true // tickDetection は mpclient=nil で detected=false を返すため、gate を直接解除
	s.mu.Unlock()

	// tickDetection を直接呼ぶ
	s.tickDetection()

	// Supervisor.mode (内部状態) 確認
	s.mu.Lock()
	internalMode := s.mode
	s.mu.Unlock()
	if internalMode != CameraModeCamera {
		t.Errorf("internal mode after tickDetection = %d, want %d (CameraModeCamera)",
			internalMode, CameraModeCamera)
	}

	// stateObserver.mode (公開状態) 確認
	if got := s.Mode(); got != CameraModeCamera {
		t.Errorf("Mode() after tickDetection = %d, want %d (CameraModeCamera)", got, CameraModeCamera)
	}
}

// TestSupervisor_BlinkFilter_InitialState は supervisor 生成時に BlinkFilter が Open 初期状態であることを確認。
func TestSupervisor_BlinkFilter_InitialState(t *testing.T) {
	s := NewSupervisor(nil, nil)
	if s.blinkFilter == nil {
		t.Fatal("blinkFilter = nil, want initialized")
	}
	if got := s.blinkFilter.State(); got != BlinkOpen {
		t.Fatalf("blinkFilter.State() = %d, want %d (BlinkOpen)", got, BlinkOpen)
	}
}

// TestSupervisor_BlinkFilter_Update_Transitions は tickCell が BlinkFilter.Update を使うことを確認。
func TestSupervisor_BlinkFilter_Update_Transitions(t *testing.T) {
	s := NewSupervisor(nil, nil)
	dr := DetectionResult{FaceDetected: true, EarLeft: 0.19, EarRight: 0.19}
	s.tickCell(dr, true)
	if got := s.blinkFilter.State(); got != BlinkHalf {
		t.Fatalf("blinkFilter.State() after 0.19 = %d, want %d (BlinkHalf)", got, BlinkHalf)
	}
	if s.EyesClosed() {
		t.Fatal("EyesClosed() after Half = true, want false")
	}
	dr.EarLeft = 0.09
	dr.EarRight = 0.09
	s.tickCell(dr, true)
	if got := s.blinkFilter.State(); got != BlinkClosed {
		t.Fatalf("blinkFilter.State() after 0.09 = %d, want %d (BlinkClosed)", got, BlinkClosed)
	}
	if !s.EyesClosed() {
		t.Fatal("EyesClosed() after Closed = false, want true")
	}
}

// TestSupervisor_BlinkFilter_Reset_OnStop は Stop 後に BlinkFilter が Open に戻ることを確認。
func TestSupervisor_BlinkFilter_Reset_OnStop(t *testing.T) {
	s := NewSupervisor(nil, nil)
	s.tickCell(DetectionResult{FaceDetected: true, EarLeft: 0.19, EarRight: 0.19}, true)
	s.tickCell(DetectionResult{FaceDetected: true, EarLeft: 0.09, EarRight: 0.09}, true)
	if got := s.blinkFilter.State(); got != BlinkClosed {
		t.Fatalf("setup blinkFilter.State() = %d, want %d (BlinkClosed)", got, BlinkClosed)
	}
	if err := s.Stop(); err != nil {
		t.Fatalf("Stop() = %v, want nil", err)
	}
	if got := s.blinkFilter.State(); got != BlinkOpen {
		t.Fatalf("blinkFilter.State() after Stop = %d, want %d (BlinkOpen)", got, BlinkOpen)
	}
}

// TestSupervisor_BlinkFilter_Reset_OnSwitchToMouse は mouse fallback 時に blink state を引き継がないことを確認。
func TestSupervisor_BlinkFilter_Reset_OnSwitchToMouse(t *testing.T) {
	s := NewSupervisor(nil, nil)
	s.tickCell(DetectionResult{FaceDetected: true, EarLeft: 0.19, EarRight: 0.19}, true)
	s.tickCell(DetectionResult{FaceDetected: true, EarLeft: 0.09, EarRight: 0.09}, true)
	s.mu.Lock()
	s.switchToMouseLocked()
	s.mu.Unlock()
	if got := s.blinkFilter.State(); got != BlinkOpen {
		t.Fatalf("blinkFilter.State() after switchToMouseLocked = %d, want %d (BlinkOpen)", got, BlinkOpen)
	}
}

// TestSupervisor_MPServer_NotStarted_DefaultState は起動前 mp_server.py 管理状態を確認。
func TestSupervisor_MPServer_NotStarted_DefaultState(t *testing.T) {
	s := NewSupervisor(nil, nil)
	if s.mpServerCmd != nil {
		t.Fatalf("mpServerCmd = %#v, want nil", s.mpServerCmd)
	}
	if s.mpServerEnabled.Load() {
		t.Fatal("mpServerEnabled = true, want false")
	}
}

// TestSupervisor_MPServer_Stop_NeverStarted_NoPanic は未起動 stopMPServer が panic しないことを確認。
func TestSupervisor_MPServer_Stop_NeverStarted_NoPanic(t *testing.T) {
	s := NewSupervisor(nil, nil)
	if err := s.stopMPServer(); err != nil {
		t.Fatalf("stopMPServer() before start = %v, want nil", err)
	}
}

// TestSupervisor_MPServer_MaxFails_SetLastError は 5回失敗後に manual restart 要求を記録することを確認。
func TestSupervisor_MPServer_MaxFails_SetLastError(t *testing.T) {
	s := NewSupervisor(nil, nil)
	s.mpServerRetry = true
	s.mpServerEnabled.Store(true)
	s.mpServerFails = mpServerMaxFails
	if err := s.monitorMPServer(); err != nil {
		t.Fatalf("monitorMPServer() = %v, want nil", err)
	}
	want := "mp_server.py 5回連続失敗、手動再起動必要"
	lastErr := s.LastError()
	if lastErr == nil || *lastErr != want {
		got := "<nil>"
		if lastErr != nil {
			got = *lastErr
		}
		t.Fatalf("LastError() = %q, want %q", got, want)
	}
}

// TestSupervisor_RestartMPServer_PreservesLastErrorOnFailure は Restart 失敗時に lastErr を残すことを検証する。
//
// Phase 2.8.1: clearLastErrorLocked が startMPServerLocked より前に動く regression を検出する。
func TestSupervisor_RestartMPServer_PreservesLastErrorOnFailure(t *testing.T) {
	s := NewSupervisor(nil, nil)
	s.mpServerRetry = true
	// NUL を含むパスは exec.Command.Start が失敗するため、実 Python / ZMQ 環境に依存しない。
	s.mpServerPath = "bad\x00mp_server.py"

	err := s.RestartMPServer()
	if err == nil {
		t.Fatal("RestartMPServer should fail with invalid mp_server.py path")
	}
	lastErr := s.LastError()
	if lastErr == nil {
		t.Fatal("LastError() should preserve failure reason after failed restart")
	}
	if !strings.Contains(*lastErr, "manual restart failed") {
		t.Fatalf("LastError should contain 'manual restart failed', got: %v", *lastErr)
	}
}

// TestSupervisor_MPServerRunning_DefaultFalse は新規 Supervisor で mp_server.py が動作していないことを検証する。
func TestSupervisor_MPServerRunning_DefaultFalse(t *testing.T) {
	s := NewSupervisor(nil, nil)
	if s.MPServerRunning() {
		t.Fatal("NewSupervisor should not have running mp_server.py")
	}
}

// TestSupervisor_LastError_AtomicNilByDefault は新規 Supervisor で LastError() が nil を返すことを検証する。
func TestSupervisor_LastError_AtomicNilByDefault(t *testing.T) {
	s := NewSupervisor(nil, nil)
	if s.LastError() != nil {
		t.Fatalf("NewSupervisor LastError() should be nil, got: %v", s.LastError())
	}
}

// === Phase 2.10.6: startup warm-up gate tests ===

// TestSupervisor_WarmupGate_BlocksImmediateSwitch は起動直後に顔検出が来ても
// warm-up gate が Mouse→Camera 切替をブロックすることを確認。
//
// tickDetectionSnapshot を DetectionResult で直接呼び、
// faceDetectionWarmupTicks 回未満では mode が CameraModeMouse のままであることを検証。
func TestSupervisor_WarmupGate_BlocksImmediateSwitch(t *testing.T) {
	s := NewSupervisor(nil, nil)
	// 起動しない (supervisor loop と race させない)

	if got := s.Mode(); got != CameraModeMouse {
		t.Fatalf("initial Mode() = %d, want %d", got, CameraModeMouse)
	}

	// 顔検出シグナルを 5 tick 分送る (warmup threshold 15 未満)
	dr := DetectionResult{FaceDetected: true}
	for i := 0; i < 5; i++ {
		s.tickDetectionSnapshot(dr, true)
	}

	// warm-up 未完了 → まだ Mouse mode
	if got := s.Mode(); got != CameraModeMouse {
		t.Errorf("Mode() after 5 detection ticks = %d, want %d (warmup not complete)", got, CameraModeMouse)
	}
}

// TestSupervisor_WarmupGate_AllowsSwitchAfterThreshold は連続検出が threshold に達した後に
// Mouse→Camera 切替が許可されることを確認。
func TestSupervisor_WarmupGate_AllowsSwitchAfterThreshold(t *testing.T) {
	s := NewSupervisor(nil, nil)

	// 顔検出シグナルを warmup threshold (15) 分連続で送る
	dr := DetectionResult{FaceDetected: true}
	for i := 0; i < faceDetectionWarmupTicks; i++ {
		s.tickDetectionSnapshot(dr, true)
	}

	// warm-up 完了 → Camera mode に切り替わる
	if got := s.Mode(); got != CameraModeCamera {
		t.Errorf("Mode() after %d detection ticks = %d, want %d (warmup complete)",
			faceDetectionWarmupTicks, got, CameraModeCamera)
	}
}

// TestSupervisor_WarmupGate_BypassedAfterWarmupDone は一度 Camera mode に入った後、
// Mouse→Camera 切替が warm-up gate をバイパスすることを確認。
// (通常の recovery speed に影響しないことの検証)
func TestSupervisor_WarmupGate_BypassedAfterWarmupDone(t *testing.T) {
	s := NewSupervisor(nil, nil)

	// Phase 1: warm-up 完了まで検出を続ける
	dr := DetectionResult{FaceDetected: true}
	for i := 0; i < faceDetectionWarmupTicks; i++ {
		s.tickDetectionSnapshot(dr, true)
	}
	if got := s.Mode(); got != CameraModeCamera {
		t.Fatalf("Mode() after warmup = %d, want %d", got, CameraModeCamera)
	}

	// Phase 2: Mouse mode にフォールバック (顔喪失)
	// lastDetected を過去に設定して timeout (1s) を超過させる。
	// faceLostTicks を grace 閾値より大きくして、最初の tick で即 faceDetected=false にする。
	s.mu.Lock()
	s.lastDetected = float64(time.Now().UnixNano())/1e9 - 2.0 // 2秒前
	s.faceLostTicks = 100 // grace (5) を超過 → 即座に faceDetected=false
	s.faceDetectedTicks = 0
	s.mu.Unlock()
	drLost := DetectionResult{FaceDetected: false}
	s.tickDetectionSnapshot(drLost, true)
	if got := s.Mode(); got != CameraModeMouse {
		t.Fatalf("Mode() after face loss = %d, want %d", got, CameraModeMouse)
	}

	// Phase 3: 顔復帰 — warmupDone=true なので 1 tick で Camera に戻る
	s.tickDetectionSnapshot(dr, true)
	if got := s.Mode(); got != CameraModeCamera {
		t.Errorf("Mode() after 1 recovery tick = %d, want %d (warmup bypassed)", got, CameraModeCamera)
	}
}

// TestSupervisor_Stop_ResetsWarmupState は Stop 後に warm-up 状態がリセットされることを確認。
func TestSupervisor_Stop_ResetsWarmupState(t *testing.T) {
	s := NewSupervisor(nil, nil)

	// warm-up を完了させる
	dr := DetectionResult{FaceDetected: true}
	for i := 0; i < faceDetectionWarmupTicks; i++ {
		s.tickDetectionSnapshot(dr, true)
	}
	if got := s.Mode(); got != CameraModeCamera {
		t.Fatalf("Mode() after warmup = %d, want %d", got, CameraModeCamera)
	}

	// Stop でリセット
	if err := s.Stop(); err != nil {
		t.Fatalf("Stop() = %v", err)
	}

	// warmupDone / faceDetectedTicks がリセットされているか確認
	s.mu.Lock()
	warmupDone := s.warmupDone
	faceDetectedTicks := s.faceDetectedTicks
	s.mu.Unlock()

	if warmupDone {
		t.Error("warmupDone should be false after Stop()")
	}
	if faceDetectedTicks != 0 {
		t.Errorf("faceDetectedTicks = %d, want 0 after Stop()", faceDetectedTicks)
	}
}

// TestSupervisor_WarmupGate_ResetOnFaceLoss は顔検出途切れ時に
// 連続カウントがリセットされることを確認。
func TestSupervisor_WarmupGate_ResetOnFaceLoss(t *testing.T) {
	s := NewSupervisor(nil, nil)

	// 10 tick 検出 (threshold 15 未満)
	dr := DetectionResult{FaceDetected: true}
	for i := 0; i < 10; i++ {
		s.tickDetectionSnapshot(dr, true)
	}

	// 途切れ
	s.tickDetectionSnapshot(DetectionResult{FaceDetected: false}, true)

	// カウントがリセットされているか確認
	s.mu.Lock()
	ticks := s.faceDetectedTicks
	s.mu.Unlock()
	if ticks != 0 {
		t.Errorf("faceDetectedTicks after face loss = %d, want 0", ticks)
	}

	// もう一度 threshold まで検出が必要
	for i := 0; i < faceDetectionWarmupTicks; i++ {
		s.tickDetectionSnapshot(dr, true)
	}
	if got := s.Mode(); got != CameraModeCamera {
		t.Errorf("Mode() after re-warmup = %d, want %d", got, CameraModeCamera)
	}
}
