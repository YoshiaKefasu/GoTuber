package camera

import "testing"

// テスト方針: 純粋関数のみ。tolerance 比較は許容せず int 値は完全一致、float64 値
// は等値比較 (テスト入力は round-trip 誤差が出ない数値を選ぶ)。
//
// Phase 2.4: `//go:build camera` を付けないため、`go test ./...` (Phase 1 ビルド) で
// 必ず実行される。capture.go / mpclient.go の integration テストは `-tags camera`
// 必要なので Phase 2.10 で別途実施予定。

// --- YawToCol ---

// TestYawToCol_Boundaries は yaw 角の主要境界・中央・クランプを検証する。
//   - 端: -30° → 0, +30° → 4
//   - 中央: 0° → 2
//   - クランプ: -60° → 0, +60° → 4 (範囲外は端に張り付く)
//   - 中央寄り: -15° → 1, +15° → 3
func TestYawToCol_Boundaries(t *testing.T) {
	cases := []struct {
		name string
		yaw  float64
		want int
	}{
		{"leftmost_-30", -30.0, 0},
		{"center_0", 0.0, 2},
		{"rightmost_+30", 30.0, 4},
		{"clamp_-60", -60.0, 0},
		{"clamp_+60", 60.0, 4},
		{"left_of_center_-15", -15.0, 1},
		{"right_of_center_+15", 15.0, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := YawToCol(tc.yaw)
			if got != tc.want {
				t.Errorf("YawToCol(%v) = %d, want %d", tc.yaw, got, tc.want)
			}
		})
	}
}

// --- PitchToRow ---

// TestPitchToRow_Boundaries は pitch 角の主要境界・中央・クランプを検証する。
//   - 端: -20° → 4 (下), +20° → 0 (上) — Y軸反転
//   - 中央: 0° → 2
//   - クランプ: -40° → 4, +40° → 0
//   - 中央寄り: -10° → 3, +10° → 1
func TestPitchToRow_Boundaries(t *testing.T) {
	cases := []struct {
		name  string
		pitch float64
		want  int
	}{
		{"down_-20", -20.0, 4},
		{"center_0", 0.0, 2},
		{"up_+20", 20.0, 0},
		{"clamp_-40", -40.0, 4},
		{"clamp_+40", 40.0, 0},
		{"down_of_center_-10", -10.0, 3},
		{"up_of_center_+10", 10.0, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := PitchToRow(tc.pitch)
			if got != tc.want {
				t.Errorf("PitchToRow(%v) = %d, want %d", tc.pitch, got, tc.want)
			}
		})
	}
}

// --- EARToBlink ---

// TestEARToBlink_Open は BlinkOpen 判定ケースを検証する。
//   - 0.22 (境界) → Open
//   - 0.30 (典型的な開眼) → Open
//   - 0.50 (上限ジャスト、範囲内) → Open
//   - -0.1 (範囲外) → Open (fallback)
func TestEARToBlink_Open(t *testing.T) {
	cases := []struct {
		name     string
		left     float64
		right    float64
		expected BlinkState
	}{
		{"boundary_open_0.22", 0.22, 0.22, BlinkOpen},
		{"typical_0.30", 0.30, 0.30, BlinkOpen},
		{"max_0.50", 0.50, 0.50, BlinkOpen},
		{"negative_fallback", -0.1, -0.1, BlinkOpen},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := EARToBlink(tc.left, tc.right)
			if got != tc.expected {
				t.Errorf("EARToBlink(%v, %v) = %v, want %v", tc.left, tc.right, got, tc.expected)
			}
		})
	}
}

// TestEARToBlink_Closed は BlinkClosed 判定ケースを検証する。
//   - 0.10 (境界) → Closed
//   - 0.05 (小さい) → Closed
//   - 0.00 (ゼロ) → Closed
func TestEARToBlink_Closed(t *testing.T) {
	cases := []struct {
		name     string
		left     float64
		right    float64
		expected BlinkState
	}{
		{"boundary_closed_0.10", 0.10, 0.10, BlinkClosed},
		{"small_0.05", 0.05, 0.05, BlinkClosed},
		{"zero_0.00", 0.00, 0.00, BlinkClosed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := EARToBlink(tc.left, tc.right)
			if got != tc.expected {
				t.Errorf("EARToBlink(%v, %v) = %v, want %v", tc.left, tc.right, got, tc.expected)
			}
		})
	}
}

// TestEARToBlink_Half は BlinkHalf 判定ケースを検証する。
//   - 0.15 (中央値) → Half
//   - 0.11 (closed 境界の真上) → Half
//   - 0.21 (open 境界の真下) → Half
func TestEARToBlink_Half(t *testing.T) {
	cases := []struct {
		name     string
		left     float64
		right    float64
		expected BlinkState
	}{
		{"middle_0.15", 0.15, 0.15, BlinkHalf},
		{"just_above_closed_0.11", 0.11, 0.11, BlinkHalf},
		{"just_below_open_0.21", 0.21, 0.21, BlinkHalf},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := EARToBlink(tc.left, tc.right)
			if got != tc.expected {
				t.Errorf("EARToBlink(%v, %v) = %v, want %v", tc.left, tc.right, got, tc.expected)
			}
		})
	}
}

// TestEARToBlink_Asymmetry は左右非対称な EAR 入力でも平均値で正しく分類される
// ことを確認する (実運用で片目だけ眩しい / MediaPipe 片側のみ confidence 高いケースを想定)。
// 平均 = (0.30 + 0.05) / 2 = 0.175 → 0.10 < 0.175 < 0.22 → BlinkHalf。
func TestEARToBlink_Asymmetry(t *testing.T) {
	got := EARToBlink(0.30, 0.05)
	if got != BlinkHalf {
		t.Errorf("EARToBlink(0.30, 0.05) = %v, want BlinkHalf (avg=0.175)", got)
	}
}

// --- FaceCenterToNormalized ---

// TestFaceCenterToNormalized_Center はフレーム中央の顔中心が原点 (0, 0) に
// 正規化されることを確認する (640x480 の中央 = (320, 240))。
func TestFaceCenterToNormalized_Center(t *testing.T) {
	nx, ny, ok := FaceCenterToNormalized(320, 240, 640, 480)
	if !ok {
		t.Fatalf("ok = false, want true")
	}
	if nx != 0.0 || ny != 0.0 {
		t.Errorf("FaceCenterToNormalized(320, 240, 640, 480) = (%v, %v), want (0, 0)", nx, ny)
	}
}

// TestFaceCenterToNormalized_Corners は 4 隅の正規化結果 (-1..+1) を確認する。
//   - (0, 0) → (-1, -1): 左上
//   - (640, 480) → (+1, +1): 右下
//
// 計算:
//   - (0/640)*2 - 1 = -1.0 (exact)
//   - (640/640)*2 - 1 = 1.0 (exact)
func TestFaceCenterToNormalized_Corners(t *testing.T) {
	// 左上
	nx, ny, ok := FaceCenterToNormalized(0, 0, 640, 480)
	if !ok || nx != -1.0 || ny != -1.0 {
		t.Errorf("(0, 0) on 640x480 = (%v, %v, ok=%v), want (-1, -1, true)", nx, ny, ok)
	}
	// 右下
	nx, ny, ok = FaceCenterToNormalized(640, 480, 640, 480)
	if !ok || nx != 1.0 || ny != 1.0 {
		t.Errorf("(640, 480) on 640x480 = (%v, %v, ok=%v), want (1, 1, true)", nx, ny, ok)
	}
}

// TestFaceCenterToNormalized_ZeroFrame は frameWidth または frameHeight が 0 以下の
// とき ok=false を返すことを確認する (division by zero 回避)。
func TestFaceCenterToNormalized_ZeroFrame(t *testing.T) {
	// height = 0
	nx, ny, ok := FaceCenterToNormalized(320, 240, 640, 0)
	if ok {
		t.Errorf("zero height: ok = true, want false (got nx=%v, ny=%v)", nx, ny)
	}
	if nx != 0.0 || ny != 0.0 {
		t.Errorf("zero height: nx=%v ny=%v, want (0, 0)", nx, ny)
	}
	// width = 0
	nx, ny, ok = FaceCenterToNormalized(320, 240, 0, 480)
	if ok {
		t.Errorf("zero width: ok = true, want false (got nx=%v, ny=%v)", nx, ny)
	}
	if nx != 0.0 || ny != 0.0 {
		t.Errorf("zero width: nx=%v ny=%v, want (0, 0)", nx, ny)
	}
	// 両方負 (念のため)
	nx, ny, ok = FaceCenterToNormalized(320, 240, -1, -1)
	if ok {
		t.Errorf("negative size: ok = true, want false (got nx=%v, ny=%v)", nx, ny)
	}
}

// --- FaceDetected ---

// TestFaceDetected_Fresh は「最終検出から 0.5 秒」経過で true を返すことを確認する
// (閾値 1.0 秒以内なので顔検出成功とみなす)。
func TestFaceDetected_Fresh(t *testing.T) {
	// now - lastDetected = 0.5 → true
	if !FaceDetected(100.0, 100.5) {
		t.Error("FaceDetected(100.0, 100.5) = false, want true (0.5s elapsed < 1.0s threshold)")
	}
}

// TestFaceDetected_Timeout は「最終検出から 1.5 秒」経過で false を返すことを確認する
// (閾値超過なので mouse fallback 推奨)。
func TestFaceDetected_Timeout(t *testing.T) {
	// now - lastDetected = 1.5 → false
	if FaceDetected(100.0, 101.5) {
		t.Error("FaceDetected(100.0, 101.5) = true, want false (1.5s elapsed >= 1.0s threshold)")
	}
}

// TestFaceDetected_Boundary は 1.0 秒ジャストで false を返すことを確認する
// (`<` 演算子なので境界値 1.0 は閾値超過扱い)。
func TestFaceDetected_Boundary(t *testing.T) {
	// now - lastDetected = 1.0 → false (1.0 boundary is excluded by `<`)
	if FaceDetected(100.0, 101.0) {
		t.Error("FaceDetected(100.0, 101.0) = true, want false (1.0s boundary excluded)")
	}
}

// TestFaceDetected_Negative は lastDetected > now のケース (未来時刻) でも true を
// 返すことを確認する。実運用では発生しない (MediaPipe 側は time.time() で過去秒を
// 返す) が、ゼロクロック付近や NTP 補正で論理逆転した場合の防御として境界検査する。
func TestFaceDetected_Negative(t *testing.T) {
	// now - lastDetected = -0.1 → true (負値 < 1.0 は true)
	if !FaceDetected(100.0, 99.9) {
		t.Error("FaceDetected(100.0, 99.9) = false, want true (future timestamp, diff is negative)")
	}
}
