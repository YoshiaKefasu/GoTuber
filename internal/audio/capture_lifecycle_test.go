package audio

import "testing"

// TestNewCaptureByID_InvalidDeviceID_NoLeak は不正な deviceID で error path に入った際、
// ctx 解放が正しく Uninit → Free の順で行われ panic しないことを確認する。
//
// Phase 1.14.1 (audio lifecycle fix): 旧コードでは error path ごとに ctx.Free() を
// 個別に呼んでおり、Uninit をスキップして "Free must only be called for an
// uninitialized context." 違反の余地があった。新コードは defer で両方を確実実行。
func TestNewCaptureByID_InvalidDeviceID_NoLeak(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("NewCaptureByID panicked on invalid deviceID (lifecycle bug): %v", r)
		}
	}()

	// 確実に存在しない device ID
	_, err := NewCaptureByID("definitely-not-a-real-device-id-zzzz")
	if err == nil {
		// 環境依存 (空文字への fallback や、偶然マッチ等) の可能性もあるので
		// エラー無しは t.Fatal せず t.Log で警告に留める。
		t.Logf("note: invalid deviceID unexpectedly succeeded; environment may accept any non-empty ID")
	}
}

// TestNewCaptureByID_EmptyDeviceID_NoPanic は deviceID="" (OS デフォルト) で
// 初期化が panic しないことを確認する。実機環境では成功、ヘッドレス環境では error
// だが panic はしない (devices_test.go の TestListDevices_DoesNotPanic と同方針)。
func TestNewCaptureByID_EmptyDeviceID_NoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("NewCaptureByID panicked on empty deviceID: %v", r)
		}
	}()

	_, _ = NewCaptureByID("") // エラーは許容 (環境依存)
}

// TestNewCaptureByID_SuccessThenStop_NoDoubleFree は audio capture lifecycle 修正の
// 最も重要なテスト。実機環境で NewCaptureByID が成功した Capture に対して Stop() を
// 呼んでも panic しないことを確認する。
//
// 旧コードのシナリオ:
//   1. NewCaptureByID 成功時、defer で ctx.Uninit() → ctx.Free() が走る
//   2. 返却された Capture の c.ctx は既に free 済み
//   3. Capture.Stop() が再度 c.ctx.Uninit() → c.ctx.Free() を呼ぶ
//   4. → 既に free 済み ctx への double-free で panic / ヒープ破壊
//
// 新コード:
//   1. NewCaptureByID 成功時、cleanupCtx = false で defer を no-op にする
//   2. c.ctx は生きている
//   3. Capture.Stop() が c.ctx を正しく解放
//   4. → panic なし
//
// 実機環境 (Windows/WSL/Linux desktop) でのみ有効。CI / ヘッドレス環境では skip。
func TestNewCaptureByID_SuccessThenStop_NoDoubleFree(t *testing.T) {
	c, err := NewCaptureByID("")
	if err != nil {
		t.Skipf("audio device not available, skipping lifecycle test: %v", err)
	}

	// Start できなくても lifecycle テストは価値がある (ctx は Start 前から解放管理される)
	if startErr := c.Start(); startErr != nil {
		t.Logf("warning: Start failed, will still test Stop lifecycle: %v", startErr)
	}

	// ここが核心: 旧コードでは panic する。
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Stop() panicked (likely double-free in lifecycle): %v", r)
		}
	}()
	c.Stop()

	// 二重解放チェック: 再度 Stop しても panic しない (内部で nil ガード)。
	// 旧コードでは 1 度目で panic するため、ここには到達しない。
	c.Stop()
}

// TestCapture_Stop_NilSafe は Capture{} (ゼロ値) に対して Stop() を呼んでも
// panic しないことを確認する。Phase 1.14.1 で Stop() の nil guard は既存実装。
func TestCapture_Stop_NilSafe(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Stop() on zero-value Capture panicked: %v", r)
		}
	}()
	c := &Capture{} // ctx = nil, device = nil
	c.Stop()
	// 冪等性: 再度呼んでも panic しない
	c.Stop()
}
