package audio

const (
	// attackRate はエンベロープ立ち上がり速度（1 更新あたり）。
	// 0.5 = 1 コールバックで 50% まで追従（~10ms @ 48kHz/1024 frame）
	attackRate = 0.5
	// releaseRate はエンベロープ減衰速度（1 更新あたり）。
	// 0.05 = 50% まで減衰するのに ~14 コールバック (~300ms)
	releaseRate = 0.05
)

// EnvelopeFollower は RMS を attack/release で平滑化する。
// 音声の自然な立ち上がり（速）と減衰（遅）を模倣する。
type EnvelopeFollower struct {
	current float64
}

// NewEnvelopeFollower は新しい EnvelopeFollower を作成する（初期値 0）。
func NewEnvelopeFollower() *EnvelopeFollower {
	return &EnvelopeFollower{}
}

// Update は新しい RMS 値でエンベロープを更新し、現在値を返す。
// rms > current なら attack（速）、それ以外は release（遅）を使用。
func (e *EnvelopeFollower) Update(rms float64) float64 {
	rate := releaseRate
	if rms > e.current {
		rate = attackRate
	}
	e.current += rate * (rms - e.current)
	return e.current
}

// Current は現在のエンベロープ値を返す（テスト・デバッグ用）。
func (e *EnvelopeFollower) Current() float64 {
	return e.current
}

// 口パク閾値とヒステリシス。
const (
	thresholdMouth0 = 0.05 // closed → half 遷移
	thresholdMouth1 = 0.20 // half → open 遷移
	hysteresis      = 0.02 // 状態遷移のデッドゾーン
)

// MouthState は口パク状態。
const (
	MouthClosed = 0 // 口閉じ
	MouthHalf   = 1 // 口半開
	MouthOpen   = 2 // 口全開
)

// MouthTracker はエンベロープ値を口パク状態 (0/1/2) にマップする。
// 閾値 ±hysteresis のデッドゾーンでフリッカ防止。
type MouthTracker struct {
	state int
}

// NewMouthTracker は新しい MouthTracker を作成する（初期状態: MouthClosed = int ゼロ値）。
func NewMouthTracker() *MouthTracker {
	return &MouthTracker{}
}

// Update はエンベロープ値で口パク状態を更新し、現在状態を返す。
// ヒステリシス:
//   - MouthClosed → MouthHalf: envelope > thresholdMouth0 + hysteresis (0.07)
//   - MouthHalf → MouthClosed: envelope < thresholdMouth0 - hysteresis (0.03)
//   - MouthHalf → MouthOpen:   envelope > thresholdMouth1 + hysteresis (0.22)
//   - MouthOpen → MouthHalf:   envelope < thresholdMouth1 - hysteresis (0.18)
//
// 設計判断: Open→Closed 直接遷移は無し。必ず Half 経由 (soft landing)。
// これにより、急激なエンベロープ減衰でも 1 フレーム = ~16ms の Half 中間状態を経由し、
// 視覚的な「ガクッ」とした跳びを抑制する。
func (m *MouthTracker) Update(envelope float64) int {
	switch m.state {
	case MouthClosed:
		if envelope > thresholdMouth0+hysteresis {
			m.state = MouthHalf
		}
	case MouthHalf:
		if envelope < thresholdMouth0-hysteresis {
			m.state = MouthClosed
		} else if envelope > thresholdMouth1+hysteresis {
			m.state = MouthOpen
		}
	case MouthOpen:
		if envelope < thresholdMouth1-hysteresis {
			m.state = MouthHalf
		}
	}
	return m.state
}

// State は現在の口パク状態を返す（テスト・デバッグ用）。
func (m *MouthTracker) State() int {
	return m.state
}
