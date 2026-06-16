package mouse

import (
	"math"
	"testing"
)

const eps = 0.01

// winW/winH は production の default ウィンドウサイズ (1280x720) と一致させる。
// テストロジックは math の正当性検証が目的なので size 自体は任意だが、
// コードを読んだときに「production 設定と同じ」と分かる方が保守しやすい。
const (
	testWinW = 1280
	testWinH = 720
)

// 中央 (mouse 座標)。
// 旧 640, 240 (ウィンドウ中央 640x480 の中央) → 新 640, 360 (1280x720 の中央)。
const (
	testCenterX = 640
	testCenterY = 360
)

// 右端 (mouse 座標)。testCenterX と同じ Y で X だけ右端に。
// tx=1, ty=0 になる。target (1, 0) のテスト用。
const testRightX = testWinW

// TestFollow_TargetClamp はマウスがウィンドウ外でも target が [-1, 1] にクランプされることを確認。
func TestFollow_TargetClamp(t *testing.T) {
	f := NewFollower(0.3)

	// マウスがウィンドウ右下に大きくはみ出し
	f.Update(2000, 2000, testWinW, testWinH, 0.3)
	tx, ty := f.target()
	if tx != 1 || ty != 1 {
		t.Errorf("expected target clamped to (1, 1), got (%v, %v)", tx, ty)
	}

	// マウスが負の座標
	f.Update(-100, -100, testWinW, testWinH, 0.3)
	tx, ty = f.target()
	if tx != -1 || ty != -1 {
		t.Errorf("expected target clamped to (-1, -1), got (%v, %v)", tx, ty)
	}

	// ウィンドウサイズ 0 → 何も更新しない
	beforeX, beforeY := f.target()
	f.Update(100, 100, 0, 0, 0.3)
	afterX, afterY := f.target()
	if beforeX != afterX || beforeY != afterY {
		t.Errorf("expected target unchanged when win size 0, got (%v, %v) → (%v, %v)",
			beforeX, beforeY, afterX, afterY)
	}
}

// TestFollow_SmoothingConverges は smoothing で current が target に収束することを確認。
func TestFollow_SmoothingConverges(t *testing.T) {
	f := NewFollower(0.3)

	// target を (1, 0) に
	f.Update(testRightX, testCenterY, testWinW, testWinH, 0.3)

	// 100 ステップ回す（0.3^100 ≈ 0 のため、ほぼ完全に target に到達する）
	for i := 0; i < 100; i++ {
		f.Update(testRightX, testCenterY, testWinW, testWinH, 0.3)
	}

	cx, cy := f.current()
	if cx < 1-eps {
		t.Errorf("expected cx ≈ 1, got %v", cx)
	}
	if math.Abs(cy) > eps {
		t.Errorf("expected cy ≈ 0, got %v", cy)
	}
}

// TestFollow_CellMapping はウィンドウ四隅が正しいセルにマップされることを確認。
// 元 tomari-guruguru app.jsx:60-62 の式 (Math.round((y+1)/2 * 4)) を完全再現:
// 左上 → (0, 0)、右下 → (4, 4)、中央 → (2, 2)。
// Go 版は int(math.Round(...)) で JS の Math.round と完全等価。
func TestFollow_CellMapping(t *testing.T) {
	f := NewFollower(1.0) // 即追従でテスト

	tests := []struct {
		name    string
		mx, my  int
		wantRow int
		wantCol int
	}{
		{"左上 (mouse=0,0)", 0, 0, 0, 0},
		{"中央 (mouse=640,360)", testCenterX, testCenterY, 2, 2},
		{"右下 (mouse=1280,720)", testWinW, testWinH, 4, 4},
		{"左下 (mouse=0,720)", 0, testWinH, 4, 0},
		{"右上 (mouse=1280,0)", testWinW, 0, 0, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f.Update(tt.mx, tt.my, testWinW, testWinH, 1.0)
			r, c := f.Cell()
			if r != tt.wantRow || c != tt.wantCol {
				t.Errorf("got (%d, %d), want (%d, %d)", r, c, tt.wantRow, tt.wantCol)
			}
		})
	}
}

// TestFollow_Granularity は元 tomari-guruguru app.jsx:62 (Math.round((y+1)/2 * 4)) と
// Go 版 (int(math.Round((y+1)/2 * 4))) の完全一致を確認する。
// 5 つの中心点 (-1, -0.5, 0, 0.5, 1) で双方同じセルを選ぶ。
func TestFollow_Granularity(t *testing.T) {
	tests := []struct {
		name    string
		normY   float64 // 正規化 y ([-1, 1])
		wantRow int
	}{
		{"y=-1.0 (top)", -1.0, 0},
		{"y=-0.5 (やや上)", -0.5, 1},
		{"y=0.0 (center)", 0.0, 2},
		{"y=0.5 (やや下)", 0.5, 3},
		{"y=1.0 (bottom)", 1.0, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := NewFollower(1.0)
			// normY → mouseY 変換: mouseY = (normY + 1) / 2 * testWinH
			mouseY := int((tt.normY + 1) / 2 * testWinH)
			f.Update(testCenterX, mouseY, testWinW, testWinH, 1.0)
			r, _ := f.Cell()
			if r != tt.wantRow {
				t.Errorf("normY=%v: got row=%d, want %d", tt.normY, r, tt.wantRow)
			}
		})
	}
}

// TestFollow_NoResponsiveness は responsiveness=0 で current が動かないことを確認。
func TestFollow_NoResponsiveness(t *testing.T) {
	f := NewFollower(0)

	// 100 回更新
	for i := 0; i < 100; i++ {
		f.Update(testCenterX, testCenterY, testWinW, testWinH, 0)
	}

	cx, cy := f.current()
	if cx != 0 || cy != 0 {
		t.Errorf("expected current unchanged at (0, 0), got (%v, %v)", cx, cy)
	}
}

// TestFollow_ResponsivenessClamp は範囲外の responsiveness がクランプされることを確認。
func TestFollow_ResponsivenessClamp(t *testing.T) {
	f := NewFollower(0.3)

	// Update に 2.0 を渡しても 1.0 にクランプ
	f.Update(testRightX, testCenterY, testWinW, testWinH, 2.0)
	for i := 0; i < 200; i++ {
		f.Update(testRightX, testCenterY, testWinW, testWinH, 2.0)
	}
	cx, _ := f.current()
	if cx < 1-eps {
		t.Errorf("expected cx ≈ 1 (clamped), got %v", cx)
	}
}
