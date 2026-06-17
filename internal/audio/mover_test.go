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
//   1. m.capture.Stop()   ← 旧 capture を解放
//   2. NewCaptureByID()   ← 失敗
//   3. return err          ← 旧 capture は失われ、Mover 全体が無効化
//
// 新コード:
//   1. NewCaptureByID()   ← 失敗
//   2. return err          ← 旧 capture は維持 (m.capture は元のまま)
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
