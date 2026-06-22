//go:build camera

package camera

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

// テスト方針: libzmq / webcam を import しない。FrameJSON の JSON roundtrip と
// CameraTracker の immutable config / state observer (atomic) のみ検証。
// Start / Close の integration テストは libzmq 必須のため Phase 2.10 で別途実施。

// TestFrameJSON_Roundtrip は FrameJSON が JSON marshal → unmarshal でデータ損失なく
// 往復することを確認する (docs/PHASE2.md §4.3 の wire format と struct tag の整合性)。
func TestFrameJSON_Roundtrip(t *testing.T) {
	original := FrameJSON{
		Type:   "frame",
		Seq:    42,
		Width:  640,
		Height: 480,
		Data:   base64.StdEncoding.EncodeToString([]byte("fake-jpeg-bytes")),
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var got FrameJSON
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if got != original {
		t.Errorf("roundtrip mismatch:\n  got:  %+v\n  want: %+v", got, original)
	}
	// 個別フィールドもチェック (== 演算子 fallback 時の検出感度向上)
	if got.Type != original.Type || got.Seq != original.Seq ||
		got.Width != original.Width || got.Height != original.Height ||
		got.Data != original.Data {
		t.Errorf("field-level mismatch: got %+v, want %+v", got, original)
	}
}

// TestFrameJSON_EmptyData は data フィールドが空でも marshal / unmarshal が
// 成功し、JSON 上に "data":"" が出力されることを確認する (omitempty tag なし)。
// 実環境では webcam 初期化直後や mp_server.py 未接続時のテスト ping で発生しうる。
func TestFrameJSON_EmptyData(t *testing.T) {
	original := FrameJSON{Type: "frame", Seq: 0, Width: 640, Height: 480, Data: ""}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	if !strings.Contains(string(data), `"data":""`) {
		t.Errorf("expected JSON to contain \"data\":\"\", got: %s", string(data))
	}

	var got FrameJSON
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if got.Data != "" {
		t.Errorf("Data: got %q, want empty", got.Data)
	}
}

// TestFrameJSON_LargeSeq は seq フィールドが uint64 max でも正確に encode / decode
// されることを確認する。seq の wrap シナリオ (現実的には 2^64 / 86400000 ≈ 2.1 億年)
// でも比較の正しさが崩れないことを保証。
func TestFrameJSON_LargeSeq(t *testing.T) {
	original := FrameJSON{Type: "frame", Seq: ^uint64(0), Width: 640, Height: 480, Data: "x"}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var got FrameJSON
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

// TestNewCameraTracker_Defaults は NewCameraTracker にゼロ値を渡した時に
// デフォルト (width=640, height=480, jpegQuality=75) にフォールバックすることを
// 確認する。mp_server.py 側の標準解像度 (640x480) と整合。
//
// unexported フィールド (width, height, jpegQuality) を直接読み出すため、
// テストファイルは package camera (internal test) として配置。
func TestNewCameraTracker_Defaults(t *testing.T) {
	tr := NewCameraTracker(0, 5555, 0, 0, 0)
	if tr == nil {
		t.Fatal("NewCameraTracker returned nil")
	}

	if tr.width != 640 {
		t.Errorf("width default: got %d, want 640", tr.width)
	}
	if tr.height != 480 {
		t.Errorf("height default: got %d, want 480", tr.height)
	}
	if tr.jpegQuality != 75 {
		t.Errorf("jpegQuality default: got %d, want 75", tr.jpegQuality)
	}
	if tr.cameraID != 0 {
		t.Errorf("cameraID: got %d, want 0", tr.cameraID)
	}
	if tr.framePort != 5555 {
		t.Errorf("framePort: got %d, want 5555", tr.framePort)
	}
}

// TestNewCameraTracker_QualityClamped は jpegQuality が範囲外 (≤0 または >100) の時に
// デフォルト 75 にクランプされることを確認する。
//
// JPEG quality の有効範囲 (libjpeg 仕様) は 1..100。0 以下は「未指定」の意味なので
// フォールバック。>100 は libjpeg 側で 100 にクランプされるため事前フォールバックで統一。
func TestNewCameraTracker_QualityClamped(t *testing.T) {
	cases := []struct {
		name string
		in   int
		want int
	}{
		{"zero → 75", 0, 75},
		{"negative → 75", -1, 75},
		{"large negative → 75", -1000, 75},
		{"over 100 → 75", 101, 75},
		{"way over 100 → 75", 999, 75},
		{"max int → 75", 1<<31 - 1, 75},
		{"valid low → as-is", 1, 1},
		{"valid high → as-is", 100, 100},
		{"valid mid → as-is", 75, 75},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tr := NewCameraTracker(0, 5555, 640, 480, tc.in)
			if tr.jpegQuality != tc.want {
				t.Errorf("jpegQuality(%d): got %d, want %d", tc.in, tr.jpegQuality, tc.want)
			}
		})
	}
}

// TestCameraTracker_StateDefaults は新規 tracker の observer (SentCount, IsRunning,
// LastErrorAt) がすべてデフォルト値 (0, false, zero time) を返すことを確認する。
// Start を呼ばない限り goroutine は起動しないため、これらは安全にアクセス可能。
func TestCameraTracker_StateDefaults(t *testing.T) {
	tr := NewCameraTracker(0, 5555, 640, 480, 75)
	if tr == nil {
		t.Fatal("NewCameraTracker returned nil")
	}

	if got := tr.SentCount(); got != 0 {
		t.Errorf("SentCount default: got %d, want 0", got)
	}
	if tr.IsRunning() {
		t.Error("IsRunning default: should be false (Start not called)")
	}
	if gotTime := tr.LastErrorAt(); !gotTime.IsZero() {
		t.Errorf("LastErrorAt default: got %v, want zero time.Time{}", gotTime)
	}

	// 冪等性: 2 度目も同じ値
	if got := tr.SentCount(); got != 0 {
		t.Errorf("SentCount 2nd call: got %d, want 0", got)
	}
	if tr.IsRunning() {
		t.Error("IsRunning 2nd call: should still be false")
	}
}
