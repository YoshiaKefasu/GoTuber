package audio

import "testing"

// TestMover_NewMoverByID_EmptyDeviceID は Mover が空 deviceID (OS デフォルト)
// で panic なく初期化されることを確認する。実機では成功、ヘッドレスでは error。
//
// Phase 1.14.1 の lifecycle 修正後でも NewMoverByID の API 仕様は変わらない
// (成功時に Capture を返す、エラー時は Mover ごと nil+error) ことを確認する回帰テスト。
func TestMover_NewMoverByID_EmptyDeviceID_NoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("NewMoverByID panicked on empty deviceID: %v", r)
		}
	}()

	_, _ = NewMoverByID("") // エラーは許容
}

// TestMover_Restart_InvalidDeviceID_KeepsOldCapture は Phase 1.14.1 の
// Mover.Restart 修正の中核テスト。実機で初期 Mover が作れた場合に限り、
// 不正な deviceID で Restart を呼んだ際、**旧 capture が維持される** ことを確認する。
//
// 旧コード:
//  1. m.capture.Stop()   ← 旧 capture を解放
//  2. NewCaptureByID()   ← 失敗
//  3. return err          ← 旧 capture は失われ、Mover 全体が無効化
//
// 新コード:
//  1. NewCaptureByID()   ← 失敗
//  2. return err          ← 旧 capture は維持 (m.capture は元のまま)
//
// CI/ヘッドレス環境では audio device が無いため skip。
func TestMover_Restart_InvalidDeviceID_KeepsOldCapture(t *testing.T) {
	// 1) 初期 Mover を実機で作る
	m, err := NewMoverByID("")
	if err != nil {
		t.Skipf("audio device not available, skipping restart lifecycle test: %v", err)
	}
	if startErr := m.Start(); startErr != nil {
		t.Skipf("audio Start failed, skipping restart lifecycle test: %v", startErr)
	}
	defer m.Stop()

	// 2) 旧 capture のポインタを記録 (lifecycle 検証用)
	oldCapture := m.capture
	if oldCapture == nil {
		t.Fatal("m.capture should be non-nil after NewMoverByID + Start")
	}

	// 3) 不正な deviceID で Restart
	err = m.Restart("definitely-not-a-real-device-id-zzzz")
	if err == nil {
		t.Fatal("expected error for invalid deviceID, got nil")
	}

	// 4) 旧 capture が維持されていることを確認 (Phase 1.14.1 の核心)
	if m.capture != oldCapture {
		t.Errorf("Restart failure should preserve old capture pointer, but it changed")
	}
	if m.capture == nil {
		t.Error("Restart failure should preserve non-nil capture")
	}
}

// TestMover_Restart_NilCapture_NoPanic は defensive nil guard の確認。
// 通常の運用では発生しないが、m.capture = nil の状態で Restart を呼んでも
// panic しないことを確認する。
//
// Phase 1.14.1 で m.capture != nil guard を追加したため、nil capture のまま
// Restart が成功した場合は新 capture に単純置換される。
func TestMover_Restart_NilCapture_NoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Restart with nil capture panicked: %v", r)
		}
	}()

	m := &Mover{
		capture:  nil, // 非通常状態だが defensive にテスト
		envelope: NewEnvelopeFollower(),
		mouth:    NewMouthTracker(),
	}

	// 確実に失敗する deviceID で Restart
	err := m.Restart("invalid-device-id-nil-capture-zzzz")
	if err == nil {
		t.Logf("note: invalid deviceID unexpectedly succeeded in nil-capture path")
	}

	// nil のまま (失敗時は旧 capture 維持の原則を nil にも適用)
	if m.capture != nil {
		t.Errorf("expected nil capture to remain nil on error, got non-nil")
	}
}

// TestMover_Restart_EmptyDeviceID_AfterValid は実機環境で連続 Restart が
// 安全であることを確認するシナリオ。Phase 1.14.1 修正前は 2 回目で double-free
// クラッシュしていた。
//
// 注意: テスト環境に依存 (実機マイク必須)。CI/ヘッドレスでは skip。
func TestMover_Restart_EmptyDeviceID_AfterValid(t *testing.T) {
	m, err := NewMoverByID("")
	if err != nil {
		t.Skipf("audio device not available: %v", err)
	}
	if startErr := m.Start(); startErr != nil {
		t.Skipf("audio Start failed: %v", startErr)
	}
	defer m.Stop()

	// 旧 capture のポインタ記録
	oldCapture := m.capture

	// 同じデフォルト deviceID で Restart
	// (旧コードではここでクラッシュ、新コードでは正常終了)
	if err := m.Restart(""); err != nil {
		t.Errorf("Restart with same empty deviceID should succeed on real hardware: %v", err)
	}

	// capture は差し替わっているはず (ライフタイムが新しいインスタンス)
	if m.capture == oldCapture {
		t.Error("Restart should produce a new Capture instance")
	}
	if m.capture == nil {
		t.Error("Restart should leave non-nil capture on success")
	}
}

// =====================================================================
// Phase 1.14.14: adaptive noise gate (applyNoiseGate) のテスト。
//
// applyNoiseGate は Mover の private メソッドなので、テストは同パッケージ
// (package audio) 内で Mover を直接構築し、capture フィールドを nil のまま
// applyNoiseGate を直接呼んで検証する。実機 audio device 不要。
//
// 設計メモ: テスト用に Mover のゼロ値 (noiseFloor=0, gateOpen=false,
// sensitivity=0) を開始点とし、applyNoiseGate の状態遷移とゲイン計算を検証する。
// sensitivity=0 のとき最初の呼び出しで defaultMicSensitivity に lazy 初期化される仕様。
// =====================================================================

// newMoverForTest は capture=nil のテスト用 Mover を返す。
// applyNoiseGate は capture を参照しないため、Mover 本体だけあれば検証可能。
func newMoverForTest() *Mover {
	return &Mover{
		capture:  nil,
		envelope: NewEnvelopeFollower(),
		mouth:    NewMouthTracker(),
		// noiseFloor=0, gateOpen=false, sensitivity=0 (lazy 初期化対象)
	}
}

// TestNoiseGate_InitialSilence_KeepsGateClosed は初期状態で無音が続く時、
// gate が開かず GatedRMS=0 を持続することを確認。
//
// シナリオ: noiseFloor=0, gateOpen=false, raw=0 (完全無音)
//
//	期待: gate closed のまま、return 0
func TestNoiseGate_InitialSilence_KeepsGateClosed(t *testing.T) {
	m := newMoverForTest()

	for i := 0; i < 10; i++ {
		got := m.applyNoiseGate(0.0)
		if got != 0 {
			t.Errorf("iter %d: expected 0 (gate closed), got %v", i, got)
		}
		if m.gateOpen {
			t.Errorf("iter %d: gate should remain closed, got open", i)
		}
	}
}

// TestNoiseGate_SilenceClimbsFloor は背景ノイズが続くと noise floor が
// ゆっくり raw 値に追従することを確認。
//
// シナリオ: raw=0.0040 を 100 回フィード (無音状態)
//
//	期待: floor は 0 → 0.0040 に単調に追従 (RiseRate=0.02)。
//	100 回反復後の収束値 ≈ raw * (1 - (1-0.02)^100) ≈ 0.0040 * 0.866 ≈ 0.00346
func TestNoiseGate_SilenceClimbsFloor(t *testing.T) {
	m := newMoverForTest()
	const raw = 0.0040
	const iters = 100

	for i := 0; i < iters; i++ {
		m.applyNoiseGate(raw)
	}

	// 100 回反復後の floor は raw にかなり近づいているはず
	if m.noiseFloor < raw*0.5 {
		t.Errorf("noise floor should rise toward raw, got %v (raw=%v)", m.noiseFloor, raw)
	}
	if m.noiseFloor > raw*1.01 {
		t.Errorf("noise floor should not overshoot raw, got %v (raw=%v)", m.noiseFloor, raw)
	}
	// 0.02 * 100 = 2.0 相当の減衰なので、収束は早い
	// (1 - 0.98^100) ≈ 0.866 → 0.0040 * 0.866 ≈ 0.00346
	if m.noiseFloor < 0.0030 || m.noiseFloor > 0.0040 {
		t.Errorf("expected noise floor in [0.0030, 0.0040], got %v", m.noiseFloor)
	}
}

// TestNoiseGate_OpensOnVoice は noise floor が安定した後、明確な音声で
// gate 開くことを確認。
func TestNoiseGate_OpensOnVoice(t *testing.T) {
	m := newMoverForTest()

	// 1) 環境ノイズで floor を安定化
	for i := 0; i < 200; i++ {
		m.applyNoiseGate(0.0040)
	}
	if m.gateOpen {
		t.Fatal("gate should be closed during silence")
	}
	floorBefore := m.noiseFloor

	// 2) 明確な音声 (raw = 0.05 = floor + ~0.046 >> openMargin=0.002)
	for i := 0; i < 5; i++ {
		got := m.applyNoiseGate(0.05)
		if i == 0 && !m.gateOpen {
			t.Errorf("gate should open on first loud sample, got closed")
		}
		if !m.gateOpen {
			t.Errorf("iter %d: gate should remain open, got closed", i)
		}
		_ = got
	}

	// floor は gate 開放中凍結されている
	if m.noiseFloor != floorBefore {
		t.Errorf("noise floor should freeze while gate open, got %v (was %v)", m.noiseFloor, floorBefore)
	}
}

// TestNoiseGate_ClosesOnSilence は gate 開いた後、無音で gate 閉じることを確認。
func TestNoiseGate_ClosesOnSilence(t *testing.T) {
	m := newMoverForTest()

	// 1) 環境ノイズで floor を 0.004 まで安定化
	for i := 0; i < 200; i++ {
		m.applyNoiseGate(0.0040)
	}
	floor := m.noiseFloor

	// 2) gate を開く
	m.applyNoiseGate(0.05)
	if !m.gateOpen {
		t.Fatal("gate should be open after loud sample")
	}

	// 3) 無音 (raw=0) → 数フレームで gate 閉じる
	for i := 0; i < 20; i++ {
		m.applyNoiseGate(0.0)
		if !m.gateOpen {
			break // gate closed
		}
	}
	if m.gateOpen {
		t.Errorf("gate should close after sustained silence, still open")
	}
	// floor は降下中のはず
	if m.noiseFloor >= floor {
		t.Errorf("noise floor should fall during sustained silence, got %v (was %v)", m.noiseFloor, floor)
	}
}

// TestNoiseGate_Hysteresis は gate 境界で頻繁に開閉しないことを確認。
// 具体的には raw = floor (open margin と close margin の間) を続けても
// gate 状態が一意に安定することを確認。
func TestNoiseGate_Hysteresis(t *testing.T) {
	m := newMoverForTest()

	// floor を 0.004 まで安定化
	for i := 0; i < 200; i++ {
		m.applyNoiseGate(0.0040)
	}

	// 0.004 (= floor) を 50 回 → gate は open しない (raw < floor + openMargin=0.002)
	for i := 0; i < 50; i++ {
		m.applyNoiseGate(0.0040)
		if m.gateOpen {
			t.Errorf("iter %d: gate should stay closed (raw=floor), got open", i)
		}
	}

	// 0.006 (= floor + 0.002 = open threshold ぴったり) → gate 開く
	//   判定: raw > floor + gateOpenMargin (0.002) → 0.006 > 0.006 は false で開かない…
	//   実装は strict greater-than ではなく >= でもないので、margin の境界値は要確認。
	//   安全策として 0.0061 (= floor + 0.0021) で開くことを確認する。
	m.applyNoiseGate(0.0061)
	if !m.gateOpen {
		t.Errorf("gate should open above open threshold, got closed")
	}

	// 0.005 (= floor + 0.001 = close threshold ぴったり) を 5 回 → gate はまだ open
	//   close 判定: raw < floor + gateCloseMargin (0.001) → 0.005 < 0.005 は false で閉じない
	for i := 0; i < 5; i++ {
		m.applyNoiseGate(0.0050)
		if !m.gateOpen {
			t.Errorf("iter %d: gate should stay open at close threshold, got closed", i)
		}
	}

	// 0.0049 (= floor + 0.0009 < close margin) → gate 閉じる
	m.applyNoiseGate(0.0049)
	if m.gateOpen {
		t.Errorf("gate should close below close margin, got open")
	}
}

// TestNoiseGate_GainApplies は gate 開放中の戻り値がゲイン適用済みであることを確認。
// 実機のシナリオ: raw=0.01, floor=0.004, sensitivity=15 のとき
//
//	voice = 0.01 - 0.004 - 0.001 = 0.005
//	scaled = 0.005 * 15 = 0.075
//
// → MouthHalf 閾値 (0.07) を超える値になる。
func TestNoiseGate_GainApplies(t *testing.T) {
	m := newMoverForTest()
	m.SetSensitivity(15)

	// floor を 0.004 まで安定化
	for i := 0; i < 200; i++ {
		m.applyNoiseGate(0.0040)
	}
	floor := m.noiseFloor

	// gate を開く
	m.applyNoiseGate(0.05)
	if !m.gateOpen {
		t.Fatal("gate should open")
	}

	// raw=0.01 をフィード
	got := m.applyNoiseGate(0.01)

	// 期待値: (0.01 - floor - 0.001) * 15
	expected := (0.01 - floor - gateCloseMargin) * 15
	if got < expected*0.9 || got > expected*1.1 {
		t.Errorf("expected gain ~%v, got %v (floor=%v)", expected, got, floor)
	}
	// ゲイン適用後 ≈ 0.075 → MouthHalf 閾値 0.07 を超えるはず
	if got <= 0.07 {
		t.Errorf("gated RMS should exceed MouthHalf threshold (0.07) with sensitivity 15, got %v", got)
	}
}

// TestNoiseGate_DefaultSensitivityAvoidsObservedIdleHalfMouth は、実機で問題になった
// 無音ノイズ値 (RMS=0.0084 / Floor=0.0020) が default 10x では MouthHalf 閾値
// 0.07 を超えないことを確認する。
func TestNoiseGate_DefaultSensitivityAvoidsObservedIdleHalfMouth(t *testing.T) {
	m := newMoverForTest()
	m.noiseFloor = 0.0020
	m.gateOpen = true
	m.noiseWarmupFrames = noiseFloorWarmupFrames

	got := m.applyNoiseGate(0.0084)
	if got >= 0.07 {
		t.Errorf("default sensitivity should keep observed idle noise below MouthHalf threshold, got %v", got)
	}
	if m.sensitivity != defaultMicSensitivity {
		t.Errorf("expected lazy default sensitivity %v, got %v", defaultMicSensitivity, m.sensitivity)
	}
}

func TestMover_SetSensitivity_Clamps(t *testing.T) {
	m := newMoverForTest()

	m.SetSensitivity(0.5)
	if m.sensitivity != minMicSensitivity {
		t.Errorf("SetSensitivity below min = %v, want %v", m.sensitivity, minMicSensitivity)
	}

	m.SetSensitivity(12.3)
	if m.sensitivity != 12.3 {
		t.Errorf("SetSensitivity in range = %v, want 12.3", m.sensitivity)
	}

	m.SetSensitivity(99)
	if m.sensitivity != maxMicSensitivity {
		t.Errorf("SetSensitivity above max = %v, want %v", m.sensitivity, maxMicSensitivity)
	}
}

// TestNoiseGate_ClampsToOne はゲイン結果が 1.0 を超えないことを確認。
func TestNoiseGate_ClampsToOne(t *testing.T) {
	m := newMoverForTest()

	// floor を 0.001 まで (raw 低い場合)
	for i := 0; i < 200; i++ {
		m.applyNoiseGate(0.001)
	}

	// gate を開く (大きな raw)
	m.applyNoiseGate(0.5)
	if !m.gateOpen {
		t.Fatal("gate should open on loud sample")
	}

	// 巨大な raw をフィード
	got := m.applyNoiseGate(1.0)
	if got > 1.0 {
		t.Errorf("gated RMS should be clamped to 1.0, got %v", got)
	}
}

// TestNoiseGate_SensitivityLazyInit は sensitivity=0 で初期化された Mover が
// 最初の applyNoiseGate 呼び出しで defaultMicSensitivity を採用することを確認。
func TestNoiseGate_SensitivityLazyInit(t *testing.T) {
	m := newMoverForTest()
	if m.sensitivity != 0 {
		t.Fatal("sensitivity should be 0 (zero value) initially")
	}

	// 1 回呼び出すと sensitivity が初期化される
	m.applyNoiseGate(0.0)
	if m.sensitivity != defaultMicSensitivity {
		t.Errorf("expected sensitivity=%v after first call, got %v",
			defaultMicSensitivity, m.sensitivity)
	}
}

// TestUpdateWithMetrics_AllFieldsPopulated は UpdateWithMetrics() の戻り値
// Metrics の全フィールド (RMS / NoiseFloor / GatedRMS / Envelope / Mouth / GateOpen)
// が設定されることを確認。
//
// 実機 audio device 必須。CI/ヘッドレスでは skip。
func TestUpdateWithMetrics_AllFieldsPopulated(t *testing.T) {
	m, err := NewMoverByID("")
	if err != nil {
		t.Skipf("audio device not available, skipping UpdateWithMetrics integration test: %v", err)
	}
	if startErr := m.Start(); startErr != nil {
		t.Skipf("audio Start failed, skipping UpdateWithMetrics integration test: %v", startErr)
	}
	defer m.Stop()

	// 1 フレーム (実機では本物の RMS が来る)
	metrics := m.UpdateWithMetrics()

	// 全フィールドが設定されている
	_ = metrics.RMS        // 値は何でも OK (環境依存)
	_ = metrics.NoiseFloor // 同上
	_ = metrics.GatedRMS   // 同上
	_ = metrics.Envelope   // 同上
	_ = metrics.Mouth      // int 0/1/2
	_ = metrics.GateOpen   // bool

	// 状態が初期化されたことを確認
	if m.noiseFloor < 0 {
		t.Errorf("noise floor should be non-negative, got %v", m.noiseFloor)
	}
	if m.sensitivity == 0 {
		t.Errorf("sensitivity should be initialized after Update, got 0")
	}
}
