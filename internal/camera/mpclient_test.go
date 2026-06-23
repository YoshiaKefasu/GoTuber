//go:build camera

package camera

import (
	"encoding/json"
	"strings"
	"testing"
)

// テスト方針: native 依存なし。DetectionJSON の JSON roundtrip と
// MPClient の immutable config / state observer (atomic) のみ検証。
// NewMPClient / ReceiveLoop の integration テストは loopback TCP を使うため、別途追加可能。

// --- DetectionJSON の JSON wire format テスト ---

// TestDetectionJSON_ParseValid は docs/PHASE2.md 4.3 のサンプル JSON (face_detected=true,
// yaw=-12.5, pitch=3.2 など) を Marshal → Unmarshal してデータ損失なく往復することを確認する。
// PHASE2.md 4.3 の Example の数字をそのまま使用 (Python mp_server.py の _compute_head_pose
// 出力範囲と整合)。
func TestDetectionJSON_ParseValid(t *testing.T) {
	original := DetectionJSON{
		Type:         "detection",
		Seq:          12345,
		Timestamp:    1718628000.123,
		FaceDetected: true,
		Yaw:          -12.5,
		Pitch:        3.2,
		Roll:         1.1,
		EarLeft:      0.28,
		EarRight:     0.30,
		FaceCenterX:  0.5,
		FaceCenterY:  0.2,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var got DetectionJSON
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	// フィールド毎チェック (== 演算子 fallback 時の検出感度向上、Phase 2.2 踏襲)
	if got.Type != original.Type {
		t.Errorf("Type: got %q, want %q", got.Type, original.Type)
	}
	if got.Seq != original.Seq {
		t.Errorf("Seq: got %d, want %d", got.Seq, original.Seq)
	}
	if got.Timestamp != original.Timestamp {
		t.Errorf("Timestamp: got %v, want %v", got.Timestamp, original.Timestamp)
	}
	if got.FaceDetected != original.FaceDetected {
		t.Errorf("FaceDetected: got %v, want %v", got.FaceDetected, original.FaceDetected)
	}
	if got.Yaw != original.Yaw {
		t.Errorf("Yaw: got %v, want %v", got.Yaw, original.Yaw)
	}
	if got.Pitch != original.Pitch {
		t.Errorf("Pitch: got %v, want %v", got.Pitch, original.Pitch)
	}
	if got.Roll != original.Roll {
		t.Errorf("Roll: got %v, want %v", got.Roll, original.Roll)
	}
	if got.EarLeft != original.EarLeft {
		t.Errorf("EarLeft: got %v, want %v", got.EarLeft, original.EarLeft)
	}
	if got.EarRight != original.EarRight {
		t.Errorf("EarRight: got %v, want %v", got.EarRight, original.EarRight)
	}
	if got.FaceCenterX != original.FaceCenterX {
		t.Errorf("FaceCenterX: got %v, want %v", got.FaceCenterX, original.FaceCenterX)
	}
	if got.FaceCenterY != original.FaceCenterY {
		t.Errorf("FaceCenterY: got %v, want %v", got.FaceCenterY, original.FaceCenterY)
	}
}

// TestDetectionJSON_NoFace は顔未検出時 (tools/mp_server.py:353-362 の
// build_detection_message landmarks_px is None パス) に yaw/pitch/roll/EAR/face_center
// が全て 0 埋めされることを確認する。PHASE2.md 4.3 仕様 + mp_server.py 実装の整合性。
//
// Go 側ではゼロ値で初期化された DetectionJSON が wire 形式として有効であることを保証。
func TestDetectionJSON_NoFace(t *testing.T) {
	// 顔未検出時の JSON (mp_server.py:354-361 と完全一致)
	raw := `{"type":"detection","seq":100,"timestamp":1718628000.0,"face_detected":false,"yaw":0.0,"pitch":0.0,"roll":0.0,"ear_left":0.0,"ear_right":0.0,"face_center_x":0.0,"face_center_y":0.0}`

	var dj DetectionJSON
	if err := json.Unmarshal([]byte(raw), &dj); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if dj.FaceDetected {
		t.Errorf("FaceDetected: got true, want false (no-face case)")
	}
	if dj.Yaw != 0 || dj.Pitch != 0 || dj.Roll != 0 {
		t.Errorf("head pose must be zero-filled when no face: got yaw=%v pitch=%v roll=%v",
			dj.Yaw, dj.Pitch, dj.Roll)
	}
	if dj.EarLeft != 0 || dj.EarRight != 0 {
		t.Errorf("EAR must be zero-filled when no face: got ear_left=%v ear_right=%v",
			dj.EarLeft, dj.EarRight)
	}
	if dj.FaceCenterX != 0 || dj.FaceCenterY != 0 {
		t.Errorf("face_center must be zero-filled when no face: got x=%v y=%v",
			dj.FaceCenterX, dj.FaceCenterY)
	}
}

// TestDetectionJSON_LargeSeq は seq フィールドが uint64 max でも正確に encode / decode
// されることを確認する。Phase 2.2 capture_test.go (TestFrameJSON_LargeSeq) と対称。
// seq の wrap シナリオ (現実的には 2^64 / 86400000 ≈ 2.1 億年) でも比較の正しさが
// 崩れないことを保証。
func TestDetectionJSON_LargeSeq(t *testing.T) {
	original := DetectionJSON{
		Type: "detection", Seq: ^uint64(0),
		Timestamp: 0, FaceDetected: false,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var got DetectionJSON
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if got.Seq != ^uint64(0) {
		t.Errorf("Seq: got %d, want %d (uint64 max)", got.Seq, ^uint64(0))
	}
	const expected = `"seq":18446744073709551615`
	if !strings.Contains(string(data), expected) {
		t.Errorf("expected JSON to contain %s, got: %s", expected, string(data))
	}
}

// TestDetectionJSON_Roundtrip_AllFields は PHASE2.md 4.3 の 11 フィールド全てが
// marshal → unmarshal でデータ損失なく往復することを確認する。
// 各フィールドに異なる非デフォルト値を入れて個別検出感度を上げる。
func TestDetectionJSON_Roundtrip_AllFields(t *testing.T) {
	original := DetectionJSON{
		Type:         "detection",
		Seq:          42,
		Timestamp:    1234567890.987,
		FaceDetected: true,
		Yaw:          -45.5,
		Pitch:        20.3,
		Roll:         -10.1,
		EarLeft:      0.15,
		EarRight:     0.18,
		FaceCenterX:  -0.7,
		FaceCenterY:  0.4,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var got DetectionJSON
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	// 完全一致 (Phase 2.2 TestFrameJSON_Roundtrip と同じパターン)
	if got.Type != original.Type || got.Seq != original.Seq ||
		got.Timestamp != original.Timestamp ||
		got.FaceDetected != original.FaceDetected ||
		got.Yaw != original.Yaw || got.Pitch != original.Pitch || got.Roll != original.Roll ||
		got.EarLeft != original.EarLeft || got.EarRight != original.EarRight ||
		got.FaceCenterX != original.FaceCenterX || got.FaceCenterY != original.FaceCenterY {
		t.Errorf("roundtrip mismatch:\n  got:  %+v\n  want: %+v", got, original)
	}
}

// TestDetectionJSON_RejectsBadJSON は wire 上に不正な JSON を受信した場合に
// json.Unmarshal が error を返すことを確認する (ReceiveLoop の error handling path)。
// Phase 2.10 統合時のフォールトトレランス検証の前提条件。
func TestDetectionJSON_RejectsBadJSON(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"empty string", ""},
		{"not JSON", "this is not json"},
		{"truncated", `{"type":"detection","seq":1`},
		{"trailing comma", `{"type":"detection","seq":1,}`},
		{"missing brace", `{"type":"detection"`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var dj DetectionJSON
			if err := json.Unmarshal([]byte(tc.raw), &dj); err == nil {
				t.Errorf("expected error for bad JSON, got nil (decoded: %+v)", dj)
			}
		})
	}
}

// TestValidateDetectionJSON_RejectsBadType は validateDetectionJSON が type != "detection"
// の時にエラーを返すことを確認する。将来 mp_server.py が複数 type (例: "stats", "debug")
// を publish する場合の早期 drop を保証 (Phase 2.3 防御的プログラミング)。
func TestValidateDetectionJSON_RejectsBadType(t *testing.T) {
	cases := []struct {
		name    string
		typeStr string
		wantErr bool
	}{
		{"valid detection", "detection", false},
		{"empty type", "", true},
		{"unknown type stats", "stats", true},
		{"unknown type debug", "debug", true},
		{"frame type (wrong)", "frame", true},
		{"uppercase", "DETECTION", true}, // 大文字小文字区別 (case-sensitive)
		{"with whitespace", " detection", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dj := &DetectionJSON{Type: tc.typeStr}
			err := validateDetectionJSON(dj)
			if tc.wantErr && err == nil {
				t.Errorf("expected error for type=%q, got nil", tc.typeStr)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error for type=%q: %v", tc.typeStr, err)
			}
		})
	}
}

// --- MPClient の state observer / lifecycle テスト (native 依存不要) ---

// TestMPClient_DefaultState は NewMPClient を呼ばずに直接生成した MPClient の observer
// (Latest / RecvCount / IsRunning / LastErrorAt) がすべてデフォルト値を返すことを確認する。
//
// MPClient{} のゼロ値で observer の安全性を確認。
// ReceiveLoop を起動するまでは goroutine が走らないため、observer は全て安全にアクセス可能
// (Phase 2.2 capture_test.go TestCameraTracker_StateDefaults と同パターン)。
func TestMPClient_DefaultState(t *testing.T) {
	c := &MPClient{detectionPort: 5556}
	if c == nil {
		t.Fatal("MPClient literal nil (unexpected)")
	}

	dr, seq, ok := c.Latest()
	if ok {
		t.Error("Latest.ok: should be false before first detection")
	}
	if seq != 0 {
		t.Errorf("Latest.seq: got %d, want 0", seq)
	}
	// zero-value DetectionResult の FaceDetected は false、他は 0 で OK だが念のため
	if dr.FaceDetected {
		t.Error("Latest.dr.FaceDetected: got true, want false (zero value)")
	}
	if dr.Yaw != 0 || dr.Pitch != 0 || dr.Roll != 0 {
		t.Errorf("Latest.dr pose: got yaw=%v pitch=%v roll=%v, want all zero",
			dr.Yaw, dr.Pitch, dr.Roll)
	}
	if !dr.Timestamp.IsZero() {
		t.Errorf("Latest.dr.Timestamp: got %v, want zero time.Time{}", dr.Timestamp)
	}

	if got := c.RecvCount(); got != 0 {
		t.Errorf("RecvCount default: got %d, want 0", got)
	}
	if c.IsRunning() {
		t.Error("IsRunning default: should be false (ReceiveLoop not called)")
	}
	if gotTime := c.LastErrorAt(); !gotTime.IsZero() {
		t.Errorf("LastErrorAt default: got %v, want zero time.Time{}", gotTime)
	}

	// 冪等性: 2 度目も同じ値
	if got := c.RecvCount(); got != 0 {
		t.Errorf("RecvCount 2nd call: got %d, want 0", got)
	}
	if c.IsRunning() {
		t.Error("IsRunning 2nd call: should still be false")
	}
}

// TestMPClient_Close_NeverStarted_NoPanic は ReceiveLoop を一度も起動していない MPClient
// に対して Close() を呼んでも panic しないことを確認する。
//
// native 依存なしのテスト環境でも panic しないことを保証。Close は cancel == nil ガード +
// wg.Wait() 即 return で safe。実際の TCP リソースは nil のため releaseResources は
// skip される (nil ガード)。
func TestMPClient_Close_NeverStarted_NoPanic(t *testing.T) {
	c := &MPClient{detectionPort: 5556}

	// defer + recover で panic を catch
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Close() panicked on never-started MPClient: %v", r)
		}
	}()

	if err := c.Close(); err != nil {
		t.Errorf("Close() returned error: %v", err)
	}

	// 2 度目の Close も idempotent
	if err := c.Close(); err != nil {
		t.Errorf("2nd Close() returned error: %v", err)
	}

	// Close 後の状態確認 (observer は依然として default)
	if c.IsRunning() {
		t.Error("IsRunning after Close: should be false")
	}
}
