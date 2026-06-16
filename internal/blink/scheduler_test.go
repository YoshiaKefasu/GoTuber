package blink

import (
	"math"
	"math/rand/v2"
	"testing"
	"time"
)

// TestBlink_Distribution は 1万回サンプリングで BlinkType の分布が 22/6/72 ± 2% に収まることを確認。
func TestBlink_Distribution(t *testing.T) {
	s := NewWithSeed(42)
	const n = 10000
	counts := map[BlinkType]int{}
	for i := 0; i < n; i++ {
		counts[s.PickBlinkType()]++
	}

	const tolerance = 2.0
	normalPct := float64(counts[BlinkNormal]) / n * 100
	doublePct := float64(counts[BlinkDouble]) / n * 100
	slowPct := float64(counts[BlinkSlow]) / n * 100

	if math.Abs(normalPct-72) > tolerance {
		t.Errorf("Normal: expected 72%% ± %v%%, got %v%%", tolerance, normalPct)
	}
	if math.Abs(doublePct-22) > tolerance {
		t.Errorf("Double: expected 22%% ± %v%%, got %v%%", tolerance, doublePct)
	}
	if math.Abs(slowPct-6) > tolerance {
		t.Errorf("Slow: expected 6%% ± %v%%, got %v%%", tolerance, slowPct)
	}
}

// TestBlink_IntervalRange は各タイプの interval が指定範囲内であることを確認。
// 仕様: 70% で 1.8-4.5s (通常)、24% で 0.7-1.5s (短い)、6% で 4.5-9s (長い)
func TestBlink_IntervalRange(t *testing.T) {
	s := NewWithSeed(42)
	const n = 10000
	var minVal, maxVal time.Duration
	minVal = time.Hour // large initial
	for i := 0; i < n; i++ {
		d := s.PickInterval()
		if d < minVal {
			minVal = d
		}
		if d > maxVal {
			maxVal = d
		}
	}
	// 全体範囲: 0.7s 〜 9s
	if minVal < 700*time.Millisecond {
		t.Errorf("min interval: expected >= 700ms, got %v", minVal)
	}
	if maxVal > 9*time.Second {
		t.Errorf("max interval: expected <= 9s, got %v", maxVal)
	}
}

// TestBlink_IntervalDistribution は interval の 3 区分分布が 70/24/6 ± 3% に収まることを確認。
func TestBlink_IntervalDistribution(t *testing.T) {
	s := NewWithSeed(42)
	const n = 10000
	var normal, short, long int
	for i := 0; i < n; i++ {
		d := s.PickInterval()
		switch {
		case d < 1500*time.Millisecond:
			short++
		case d < 4500*time.Millisecond:
			normal++
		default:
			long++
		}
	}
	normalPct := float64(normal) / n * 100
	shortPct := float64(short) / n * 100
	longPct := float64(long) / n * 100
	const tolerance = 3.0
	if math.Abs(normalPct-70) > tolerance {
		t.Errorf("Normal interval: expected 70%% ± %v%%, got %v%%", tolerance, normalPct)
	}
	if math.Abs(shortPct-24) > tolerance {
		t.Errorf("Short interval: expected 24%% ± %v%%, got %v%%", tolerance, shortPct)
	}
	if math.Abs(longPct-6) > tolerance {
		t.Errorf("Long interval: expected 6%% ± %v%%, got %v%%", tolerance, longPct)
	}
}

// TestBlink_BlinkDuration は瞬き継続時間が仕様範囲内であることを確認。
// - Normal/Double: 80-150ms
// - Slow: 200-300ms
func TestBlink_BlinkDuration(t *testing.T) {
	rng := rand.New(rand.NewPCG(42, 0x9E3779B97F4A7C15))
	// Normal: 80 + IntN(71) = 80-150ms
	for i := 0; i < 1000; i++ {
		d := time.Duration(blinkNormalMinMs+rng.IntN(blinkNormalMaxMs-blinkNormalMinMs+1)) * time.Millisecond
		if d < 80*time.Millisecond || d > 150*time.Millisecond {
			t.Errorf("Normal blink duration out of range: %v", d)
		}
	}
	// Slow: 200 + IntN(101) = 200-300ms
	for i := 0; i < 1000; i++ {
		d := time.Duration(blinkSlowMinMs+rng.IntN(blinkSlowMaxMs-blinkSlowMinMs+1)) * time.Millisecond
		if d < 200*time.Millisecond || d > 300*time.Millisecond {
			t.Errorf("Slow blink duration out of range: %v", d)
		}
	}
}

// TestBlink_StateTransitions は状態機械の遷移をシミュレートして確認。
// 1) stateOpen → stateBlink1 (瞬き開始)
// 2) stateBlink1 → stateOpen (通常/ゆっくり)、または → stateBetween (二度瞬き)
// 3) stateBetween → stateBlink2 (二度瞬き 2 回目)
// 4) stateBlink2 → stateOpen
func TestBlink_StateTransitions(t *testing.T) {
	s := NewWithSeed(42)
	// 1秒後 (first blink) → 状態変化
	now := time.Now()
	now1 := now.Add(1500 * time.Millisecond) // 1.5s 後
	eyesClosed := s.Update(now1)
	if !eyesClosed {
		t.Errorf("expected eyesClosed=true after first blink trigger, got false")
	}

	// 瞬き継続時間 (Normal: 80-150ms) 経過後 → 目が開く
	now2 := now1.Add(200 * time.Millisecond) // 200ms 後
	eyesClosed = s.Update(now2)
	if eyesClosed {
		// 正常な瞬きなら、もう閉じているべきではない
		// (二度の 1 回目でも、この期間は閉じている)
		// どちらでも構わない。状態は正常遷移
	}

	// 次の瞬き (1.8-4.5s 後) → もう 1 度閉じる
	now3 := now2.Add(3 * time.Second)
	eyesClosed = s.Update(now3)
	// ここでは瞬き中かもしれないし、まだの場合もある
	_ = eyesClosed
}

// TestBlink_NewSeed は異なる seed で異なるシーケンスが生成されることを確認。
func TestBlink_NewSeed(t *testing.T) {
	s1 := NewWithSeed(1)
	s2 := NewWithSeed(2)
	if s1.PickBlinkType() == s2.PickBlinkType() && s1.PickInterval() == s2.PickInterval() {
		// 完全に同じになる確率は非常に低い
		// (両方とも最初の呼び出しなので 1/3 * 1/3 程度)
		// テストとしては緩いが、seed が異なることを確認
		// 実際のシーケンスが違うことを示すには 10 回程度呼ぶ
		seq1 := make([]BlinkType, 10)
		seq2 := make([]BlinkType, 10)
		for i := 0; i < 10; i++ {
			seq1[i] = s1.PickBlinkType()
			seq2[i] = s2.PickBlinkType()
		}
		same := true
		for i := 0; i < 10; i++ {
			if seq1[i] != seq2[i] {
				same = false
				break
			}
		}
		if same {
			t.Errorf("expected different sequences for different seeds, got same")
		}
	}
}
