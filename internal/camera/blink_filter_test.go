package camera

import (
	"math"
	"testing"
)

// TestBlinkFilter_InitialState は NewBlinkFilter が BlinkOpen 初期状態を返すことを確認する。
func TestBlinkFilter_InitialState(t *testing.T) {
	filter := NewBlinkFilter()
	if got := filter.State(); got != BlinkOpen {
		t.Fatalf("State() = %v, want BlinkOpen", got)
	}
}

// TestBlinkFilter_OpenToHalf は Open 状態から下降しきい値未満で Half へ遷移することを確認する。
func TestBlinkFilter_OpenToHalf(t *testing.T) {
	filter := NewBlinkFilter()
	if got := filter.Update(0.25, 0.25); got != BlinkOpen {
		t.Fatalf("Update(0.25, 0.25) = %v, want BlinkOpen", got)
	}
	if got := filter.Update(0.19, 0.19); got != BlinkHalf {
		t.Fatalf("Update(0.19, 0.19) = %v, want BlinkHalf", got)
	}
}

// TestBlinkFilter_HalfToOpen は Half 状態から上昇しきい値超過で Open へ復帰することを確認する。
func TestBlinkFilter_HalfToOpen(t *testing.T) {
	filter := NewBlinkFilter()
	if got := filter.Update(0.15, 0.15); got != BlinkHalf {
		t.Fatalf("Update(0.15, 0.15) = %v, want BlinkHalf", got)
	}
	if got := filter.Update(0.25, 0.25); got != BlinkOpen {
		t.Fatalf("Update(0.25, 0.25) = %v, want BlinkOpen", got)
	}
}

// TestBlinkFilter_HalfToClosed は Half 状態から下降しきい値未満で Closed へ遷移することを確認する。
func TestBlinkFilter_HalfToClosed(t *testing.T) {
	filter := NewBlinkFilter()
	if got := filter.Update(0.15, 0.15); got != BlinkHalf {
		t.Fatalf("Update(0.15, 0.15) = %v, want BlinkHalf", got)
	}
	if got := filter.Update(0.09, 0.09); got != BlinkClosed {
		t.Fatalf("Update(0.09, 0.09) = %v, want BlinkClosed", got)
	}
}

// TestBlinkFilter_ClosedToHalf は Closed 状態から上昇しきい値超過で Half へ復帰することを確認する。
func TestBlinkFilter_ClosedToHalf(t *testing.T) {
	filter := NewBlinkFilter()
	if got := filter.Update(0.09, 0.09); got != BlinkHalf {
		t.Fatalf("first Update(0.09, 0.09) = %v, want BlinkHalf", got)
	}
	if got := filter.Update(0.09, 0.09); got != BlinkClosed {
		t.Fatalf("second Update(0.09, 0.09) = %v, want BlinkClosed", got)
	}
	if got := filter.Update(0.15, 0.15); got != BlinkHalf {
		t.Fatalf("Update(0.15, 0.15) = %v, want BlinkHalf", got)
	}
}

// TestBlinkFilter_HysteresisAbsorbsNoise は Open 状態の 0.20-0.24 帯ノイズが吸収されることを確認する。
func TestBlinkFilter_HysteresisAbsorbsNoise(t *testing.T) {
	filter := NewBlinkFilter()
	for _, ear := range []float64{0.25, 0.21, 0.23, 0.20} {
		if got := filter.Update(ear, ear); got != BlinkOpen {
			t.Fatalf("Update(%v, %v) = %v, want BlinkOpen", ear, ear, got)
		}
	}
	if got := filter.Update(0.19, 0.19); got != BlinkHalf {
		t.Fatalf("Update(0.19, 0.19) = %v, want BlinkHalf", got)
	}
}

// TestBlinkFilter_FullCycle は Open→Half→Closed→Half→Open のフルサイクルを確認する。
func TestBlinkFilter_FullCycle(t *testing.T) {
	filter := NewBlinkFilter()
	cases := []struct {
		name string
		ear  float64
		want BlinkState
	}{
		{"open_stable", 0.25, BlinkOpen},
		{"open_to_half", 0.19, BlinkHalf},
		{"half_to_closed", 0.09, BlinkClosed},
		{"closed_to_half", 0.15, BlinkHalf},
		{"half_to_open", 0.25, BlinkOpen},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := filter.Update(tc.ear, tc.ear); got != tc.want {
				t.Fatalf("Update(%v, %v) = %v, want %v", tc.ear, tc.ear, got, tc.want)
			}
		})
	}
}

// TestBlinkFilter_OutOfRange_Open は範囲外入力が BlinkOpen にフォールバックすることを確認する。
func TestBlinkFilter_OutOfRange_Open(t *testing.T) {
	cases := []struct {
		name  string
		left  float64
		right float64
	}{
		{"negative", -0.1, -0.1},
		{"too_large", 0.51, 0.51},
		{"nan", math.NaN(), math.NaN()},
		{"positive_inf", math.Inf(1), math.Inf(1)},
		{"negative_inf", math.Inf(-1), math.Inf(-1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			filter := NewBlinkFilter()
			if got := filter.Update(0.15, 0.15); got != BlinkHalf {
				t.Fatalf("setup Update(0.15, 0.15) = %v, want BlinkHalf", got)
			}
			if got := filter.Update(tc.left, tc.right); got != BlinkOpen {
				t.Fatalf("Update(%v, %v) = %v, want BlinkOpen", tc.left, tc.right, got)
			}
		})
	}
}

// TestBlinkFilter_Reset は Reset 後に BlinkOpen 初期状態へ戻ることを確認する。
func TestBlinkFilter_Reset(t *testing.T) {
	filter := NewBlinkFilter()
	if got := filter.Update(0.15, 0.15); got != BlinkHalf {
		t.Fatalf("Update(0.15, 0.15) = %v, want BlinkHalf", got)
	}
	filter.Reset()
	if got := filter.State(); got != BlinkOpen {
		t.Fatalf("State() after Reset() = %v, want BlinkOpen", got)
	}
}

// TestBlinkFilter_StableState_NoTransition は Open 状態では 0.20-0.24 帯を Open 維持することを確認する。
func TestBlinkFilter_StableState_NoTransition(t *testing.T) {
	filter := NewBlinkFilter()
	if got := filter.Update(0.22, 0.22); got != BlinkOpen {
		t.Fatalf("Update(0.22, 0.22) = %v, want BlinkOpen", got)
	}
}

// TestBlinkFilter_StableState_Asymmetry は左右非対称入力が平均値で Half へ遷移することを確認する。
func TestBlinkFilter_StableState_Asymmetry(t *testing.T) {
	filter := NewBlinkFilter()
	if got := filter.Update(0.25, 0.05); got != BlinkHalf {
		t.Fatalf("Update(0.25, 0.05) = %v, want BlinkHalf (avg=0.15)", got)
	}
}
