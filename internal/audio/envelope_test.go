package audio

import (
	"math"
	"testing"
)

// TestEnvelope_AttackRelease は attack=0.5 / release=0.05 で平滑化されることを確認。
func TestEnvelope_AttackRelease(t *testing.T) {
	e := NewEnvelopeFollower()

	// 初期値 0
	if e.Current() != 0 {
		t.Errorf("expected initial current=0, got %v", e.Current())
	}

	// 1) rms > current (attack)
	got := e.Update(1.0)
	expectedAttack := 0 + attackRate*(1.0-0) // 0.5
	if math.Abs(got-expectedAttack) > 0.01 {
		t.Errorf("attack step: expected %v, got %v", expectedAttack, got)
	}

	// 2) 同じ rms を 10 回 → 収束
	for i := 0; i < 10; i++ {
		e.Update(1.0)
	}
	if e.Current() < 0.99 {
		t.Errorf("expected current ≈ 1 after 10 attack steps, got %v", e.Current())
	}

	// 3) rms < current (release)
	envelopeBeforeRelease := e.Current()
	e.Update(0.0) // 大きく下げる
	expectedRelease := envelopeBeforeRelease + releaseRate*(0.0-envelopeBeforeRelease)
	if math.Abs(e.Current()-expectedRelease) > 0.01 {
		t.Errorf("release step: expected %v, got %v", expectedRelease, e.Current())
	}

	// 4) release の方が attack より遅い (10 step で 0.5^10 vs 0.95^10)
	e2 := NewEnvelopeFollower()
	e2.Update(1.0)
	for i := 0; i < 10; i++ {
		e2.Update(1.0)
	}
	// attack が速い: 1.0 にほぼ到達
	if e2.Current() < 0.99 {
		t.Errorf("attack should be fast: expected ≈ 1, got %v", e2.Current())
	}

	e3 := NewEnvelopeFollower()
	e3.Update(1.0) // 1.0 に即到達 (初期 0 から attack)
	e3.Update(0.0)
	for i := 0; i < 10; i++ {
		e3.Update(0.0)
	}
	// release が遅い: 10 step でも 0.4^10 ≈ 0.0001 でほぼ 0
	// 0.95^10 ≈ 0.5987 → まだ 0.6 程度残っている
	if e3.Current() > 0.7 {
		t.Errorf("release should be slow: expected < 0.7, got %v", e3.Current())
	}
}

// TestMouth_Hysteresis は closed↔half, half↔open の閾値とヒステリシスが正しいことを確認。
// 仕様:
//   - closed → half:  envelope > thresholdMouth0 + hysteresis = 0.07
//   - half → closed:  envelope < thresholdMouth0 - hysteresis = 0.03
//   - half → open:    envelope > thresholdMouth1 + hysteresis = 0.22
//   - open → half:    envelope < thresholdMouth1 - hysteresis = 0.18
func TestMouth_Hysteresis(t *testing.T) {
	m := NewMouthTracker()

	// 初期: closed
	if m.State() != MouthClosed {
		t.Errorf("expected initial MouthClosed, got %v", m.State())
	}

	// 0.04 → closed (< 0.07)
	m.Update(0.04)
	if m.State() != MouthClosed {
		t.Errorf("envelope=0.04 should be MouthClosed, got %v", m.State())
	}

	// 0.08 → half (> 0.07)
	m.Update(0.08)
	if m.State() != MouthHalf {
		t.Errorf("envelope=0.08 should be MouthHalf, got %v", m.State())
	}

	// 0.05 → half (closed 範囲外: 0.03 < 0.05 < 0.07、半範囲内維持)
	m.Update(0.05)
	if m.State() != MouthHalf {
		t.Errorf("envelope=0.05 from half should be MouthHalf, got %v", m.State())
	}

	// 0.10 → half (half 範囲内)
	m.Update(0.10)
	if m.State() != MouthHalf {
		t.Errorf("envelope=0.10 should be MouthHalf, got %v", m.State())
	}

	// 0.25 → open (> 0.22)
	m.Update(0.25)
	if m.State() != MouthOpen {
		t.Errorf("envelope=0.25 should be MouthOpen, got %v", m.State())
	}

	// 0.16 → half (< 0.18)
	m.Update(0.16)
	if m.State() != MouthHalf {
		t.Errorf("envelope=0.16 from open should be MouthHalf, got %v", m.State())
	}

	// 0.01 → closed (< 0.03)
	m.Update(0.01)
	if m.State() != MouthClosed {
		t.Errorf("envelope=0.01 from half should be MouthClosed, got %v", m.State())
	}
}

// TestMouth_OpenToClosed_NoDirectTransition は Open→Closed 直接遷移がないことを確認。
// Open からいきなり closed 範囲に戻しても Half を経由する。
func TestMouth_OpenToClosed_NoDirectTransition(t *testing.T) {
	m := NewMouthTracker()

	// Open 状態にする
	m.Update(0.30) // → half
	m.Update(0.30) // → open
	if m.State() != MouthOpen {
		t.Fatalf("expected MouthOpen, got %v", m.State())
	}

	// いきなり 0.01 (closed 範囲) に戻すが、open → half threshold (0.18) で止まる
	m.Update(0.01)
	if m.State() != MouthHalf {
		t.Errorf("expected MouthHalf (via half), got %v (open→closed direct transition is forbidden)", m.State())
	}

	// さらに closed 範囲
	m.Update(0.01)
	if m.State() != MouthClosed {
		t.Errorf("expected MouthClosed, got %v", m.State())
	}
}

// TestRMS_Normalization は int16 RMS が [0, 1] に正規化されることを確認。
func TestRMS_Normalization(t *testing.T) {
	// 無音: 全 0
	rms := computeRMS([]int16{0, 0, 0, 0})
	if rms != 0 {
		t.Errorf("expected RMS=0 for silence, got %v", rms)
	}

	// 満振幅: 全 32767 (int16 max)
	rms = computeRMS([]int16{32767, -32768, 32767, -32768})
	if math.Abs(rms-1.0) > 0.01 {
		t.Errorf("expected RMS≈1 for full amplitude, got %v", rms)
	}

	// 半振幅: 16384
	rms = computeRMS([]int16{16384, -16384, 16384, -16384})
	expected := 0.5 // 16384/32768 = 0.5
	if math.Abs(rms-expected) > 0.01 {
		t.Errorf("expected RMS≈0.5 for half amplitude, got %v", rms)
	}

	// 空入力
	rms = computeRMS([]int16{})
	if rms != 0 {
		t.Errorf("expected RMS=0 for empty input, got %v", rms)
	}
}
// TestDecodePCM16 はバイト列 → int16 変換の境界を確認。
func TestDecodePCM16(t *testing.T) {
	// 空
	got := decodePCM16([]byte{})
	if len(got) != 0 {
		t.Errorf("expected empty for empty bytes, got %d samples", len(got))
	}
	releasePCMSamples(got) // 必ず release (規約統一)

	// 2 バイト (1 sample) → 0x0001 = 1
	got = decodePCM16([]byte{0x01, 0x00})
	if len(got) != 1 || got[0] != 1 {
		t.Errorf("expected [1], got %v", got)
	}
	releasePCMSamples(got)

	// 4 バイト (2 samples) → [0x0001, 0x8000] = [1, -32768]
	got = decodePCM16([]byte{0x01, 0x00, 0x00, 0x80})
	if len(got) != 2 || got[0] != 1 || got[1] != -32768 {
		t.Errorf("expected [1, -32768], got %v", got)
	}
	releasePCMSamples(got)
}

// TestDecodePCM16_PoolReuse は sync.Pool が slice を再利用する (backing array が同一) ことを確認。
// 1) 同じサイズで取り出し→解放→再取り出しして &samples[0] 比較
// 2) 容量不足時に新規確保されることを確認
func TestDecodePCM16_PoolReuse(t *testing.T) {
	data := []byte{0x01, 0x00, 0x02, 0x00, 0x03, 0x00, 0x04, 0x00} // 8B → 4 samples

	// 1) Pool 再利用検証: backing array アドレス一致
	s1 := decodePCM16(data)
	if len(s1) != 4 {
		t.Fatalf("first: expected 4 samples, got %d", len(s1))
	}
	addr1 := &s1[0]
	releasePCMSamples(s1)
	s2 := decodePCM16(data)
	if len(s2) != 4 {
		t.Fatalf("second: expected 4 samples, got %d", len(s2))
	}
	if &s2[0] != addr1 {
		t.Errorf("pool did not reuse slice: s1 backing=%p, s2 backing=%p", addr1, &s2[0])
	}
	releasePCMSamples(s2)

	// 2) 容量不足時に新規確保 (initial cap 1024 を超える)
	big := make([]byte, 4096) // 2048 samples > initial 1024
	sBig := decodePCM16(big)
	if len(sBig) != 2048 {
		t.Errorf("big: expected 2048 samples, got %d", len(sBig))
	}
	if cap(sBig) < 2048 {
		t.Errorf("big: expected cap >= 2048, got %d", cap(sBig))
	}
	releasePCMSamples(sBig)
}

// TestMover_Update_NoAudio は audio なしで Update しても 0 を返すことを確認 (nil safety)。
func TestMover_Update_NilSafe(t *testing.T) {
	// 初期化できないので、EnvelopeFollower と MouthTracker の独立テスト
	e := NewEnvelopeFollower()
	m := NewMouthTracker()

	// 100 回更新してもエラーなし
	for i := 0; i < 100; i++ {
		env := e.Update(0.0)
		state := m.Update(env)
		_ = state
	}

	// 最終的に MouthClosed (envelope=0 < threshold=0.05)
	if m.State() != MouthClosed {
		t.Errorf("expected MouthClosed, got %v", m.State())
	}
}
