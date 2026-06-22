package camera

import "math"

// BlinkFilter は EAR 値にヒステリシスを被せて 3 状態 (Open/Half/Closed) を返すフィルタ。
//
// Phase 2.4 の EARToBlink は単純な 2 段しきい値 (Open 0.22 / Closed 0.10) で
// ノイズにより状態遷移が細かく揺れる可能性がある。BlinkFilter は状態遷移に
// ヒステリシスを適用し、下降時と上昇時で異なる閾値を使うことで細かい
// ノイズを吸収する (デバウンス効果)。
//
// ヒステリシス仕様 (docs/PHASE2.md Section 2.6 で確定):
//   - Open → Half:   earAvg < 0.20 (下降しきい値)
//   - Half → Open:   earAvg > 0.24 (上昇しきい値)
//   - Half → Closed: earAvg < 0.10 (下降しきい値)
//   - Closed → Half: earAvg > 0.14 (上昇しきい値)
//
// ヒステリシス幅は Open↔Half=0.04、Half↔Closed=0.04 で細かいノイズを吸収。
// 状態は閉じ方向 (Open→Half→Closed) と開き方向 (Closed→Half→Open) で
// 異なる閾値を使う classic ヒステリシス。
//
// Phase 2.4 mapper.EARToBlink と異なり、BlinkFilter は stateful (前回状態を保持)
// ため、フィルタインスタンスは supervisor (Phase 2.5) や game.go から
// 60Hz 程度の頻度で Update() を呼ぶ想定。
//
// 初期状態: BlinkOpen (閉眼状態なし、開眼状態から開始)。
// mutex は持たない。Phase 2.5 supervisor loop など 1 goroutine から呼ぶ想定で、
// 複数 goroutine 共有が必要になった時点で呼び出し側か本型に同期を追加する。
//
// Phase 2.6: ヒステリシス実装、別レイヤ被せ。
type BlinkFilter struct {
	state     BlinkState
	lastInput float64
}

const (
	earFilterOpenToHalf   = 0.20 // 下降 (Open → Half)
	earFilterHalfToOpen   = 0.24 // 上昇 (Half → Open)
	earFilterHalfToClosed = 0.10 // 下降 (Half → Closed)
	earFilterClosedToHalf = 0.14 // 上昇 (Closed → Half)
)

// averageEAR は左右 EAR を単純平均する。
//
// Phase 2.4 の EARToBlink と同じ集約方法に揃え、BlinkFilter は「判定レイヤ」だけを
// 差し替える。片目だけ眩しい / 片側 landmark の揺れがあるケースも Phase 2.4 と同様に
// 平均値で代表させる。
func averageEAR(earLeft, earRight float64) float64 {
	return (earLeft + earRight) / 2.0
}

// invalidEAR は BlinkOpen にフォールバックすべき不正 EAR 値かを返す。
//
// Phase 2.4 の範囲外 (<0 / >0.5) fallback と整合させつつ、NaN / Inf は比較演算だけでは
// 捕捉しづらいため明示的に不正値扱いする。
func invalidEAR(earAvg float64) bool {
	return earAvg < 0 || earAvg > 0.5 || math.IsNaN(earAvg) || math.IsInf(earAvg, 0)
}

// NewBlinkFilter は BlinkFilter を BlinkOpen 初期状態で生成する。
func NewBlinkFilter() *BlinkFilter {
	return &BlinkFilter{state: BlinkOpen}
}

// Update は左右 EAR 平均値 (0.0-0.5) を入力としてヒステリシス適用後の
// BlinkState を返す。state を内部で更新する (stateful)。
//
// 範囲外 (< 0 || > 0.5) は BlinkOpen にフォールバックする (Phase 2.4 と同じ安全側)。
// NaN / Inf も MediaPipe noise 由来の不正値として BlinkOpen にフォールバックする。
func (f *BlinkFilter) Update(earLeft, earRight float64) BlinkState {
	earAvg := averageEAR(earLeft, earRight)
	f.lastInput = earAvg

	if invalidEAR(earAvg) {
		f.state = BlinkOpen
		return f.state
	}

	switch f.state {
	case BlinkOpen:
		if earAvg < earFilterOpenToHalf {
			f.state = BlinkHalf
		}
	case BlinkHalf:
		if earAvg < earFilterHalfToClosed {
			f.state = BlinkClosed
		} else if earAvg > earFilterHalfToOpen {
			f.state = BlinkOpen
		}
	case BlinkClosed:
		if earAvg > earFilterClosedToHalf {
			f.state = BlinkHalf
		}
	default:
		f.state = BlinkOpen
	}

	return f.state
}

// State は現在の BlinkState を返す (atomic 読み出しなし、純粋読み取り)。
func (f *BlinkFilter) State() BlinkState {
	return f.state
}

// Reset は BlinkFilter を BlinkOpen 初期状態にリセットする (テスト用途)。
func (f *BlinkFilter) Reset() {
	f.state = BlinkOpen
	f.lastInput = 0
}
