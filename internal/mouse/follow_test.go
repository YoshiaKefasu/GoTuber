package mouse

import (
	"math"
	"testing"
)

const eps = 0.01

// TestFollow_TargetClamp はマウスがウィンドウ外でも target が [-1, 1] にクランプされることを確認。
func TestFollow_TargetClamp(t *testing.T) {
	f := NewFollower(0.3)

	// マウスがウィンドウ右下に大きくはみ出し
	f.Update(1000, 1000, 640, 480)
	tx, ty := 	f.target()
	if tx != 1 || ty != 1 {
		t.Errorf("expected target clamped to (1, 1), got (%v, %v)", tx, ty)
	}

	// マウスが負の座標
	f.Update(-100, -100, 640, 480)
	tx, ty = 	f.target()
	if tx != -1 || ty != -1 {
		t.Errorf("expected target clamped to (-1, -1), got (%v, %v)", tx, ty)
	}

	// ウィンドウサイズ 0 → 何も更新しない
	beforeX, beforeY := 	f.target()
	f.Update(100, 100, 0, 0)
	afterX, afterY := 	f.target()
	if beforeX != afterX || beforeY != afterY {
		t.Errorf("expected target unchanged when win size 0, got (%v, %v) → (%v, %v)",
			beforeX, beforeY, afterX, afterY)
	}
}

// TestFollow_SmoothingConverges は smoothing で current が target に収束することを確認。
func TestFollow_SmoothingConverges(t *testing.T) {
	f := NewFollower(0.3)

	// target を (1, 0) に
	f.Update(640, 240, 640, 480)

	// 100 ステップ回す（0.3^100 ≈ 0 のため、ほぼ完全に target に到達する）
	for i := 0; i < 100; i++ {
		f.Update(640, 240, 640, 480)
	}

	cx, cy := 	f.current()
	if cx < 1-eps {
		t.Errorf("expected cx ≈ 1, got %v", cx)
	}
	if math.Abs(cy) > eps {
		t.Errorf("expected cy ≈ 0, got %v", cy)
	}
}

// TestFollow_CellMapping はウィンドウ四隅が正しいセルにマップされることを確認。
// Y 軸反転のため: 左上 → (4, 0)、右下 → (0, 4)、中央 → (2, 2)。
func TestFollow_CellMapping(t *testing.T) {
	f := NewFollower(1.0) // 即追従でテスト

	tests := []struct {
		name        string
		mx, my      int
		wantRow     int
		wantCol     int
	}{
		{"左上 (mouse=0,0)", 0, 0, 4, 0},
		{"中央 (mouse=320,240)", 320, 240, 2, 2},
		{"右下 (mouse=640,480)", 640, 480, 0, 4},
		{"左下 (mouse=0,480)", 0, 480, 0, 0},
		{"右上 (mouse=640,0)", 640, 0, 4, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f.Update(tt.mx, tt.my, 640, 480)
			r, c := f.Cell()
			if r != tt.wantRow || c != tt.wantCol {
				t.Errorf("got (%d, %d), want (%d, %d)", r, c, tt.wantRow, tt.wantCol)
			}
		})
	}
}

// TestFollow_NoResponsiveness は responsiveness=0 で current が動かないことを確認。
func TestFollow_NoResponsiveness(t *testing.T) {
	f := NewFollower(0)

	// 100 回更新
	for i := 0; i < 100; i++ {
		f.Update(640, 240, 640, 480)
	}

	cx, cy := 	f.current()
	if cx != 0 || cy != 0 {
		t.Errorf("expected current unchanged at (0, 0), got (%v, %v)", cx, cy)
	}
}

// TestFollow_ResponsivenessClamp は範囲外の responsiveness がクランプされることを確認。
func TestFollow_ResponsivenessClamp(t *testing.T) {
	tests := []struct {
		name string
		in   float64
		want float64
	}{
		{"負の値 → 0", -0.5, 0},
		{"0 → 0", 0, 0},
		{"0.5 → 0.5", 0.5, 0.5},
		{"1.0 → 1.0", 1.0, 1.0},
		{"2.0 → 1.0", 2.0, 1.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := NewFollower(tt.in)
			if f.responsiveness != tt.want {
				t.Errorf("got %v, want %v", f.responsiveness, tt.want)
			}
		})
	}
}
