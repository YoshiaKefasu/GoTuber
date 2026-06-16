// Package blink は自動まばたきを制御する。
//
// 瞬きタイプ分布: 22% 二度瞬き / 6% ゆっくり / 72% 通常
// 瞬き間隔範囲: 1.8〜4.5s (70% 未満, 通常) / 0.7〜1.5s (24% 未満, たまに短く) / 4.5〜9s (6%, ぼーっと長く)
// 瞬き継続時間: 通常 80〜150ms / ゆっくり 200〜300ms
// 二度瞬き: 1 回目 80〜150ms → 50〜100ms 開く → 2 回目 80〜150ms
//
// 状態遷移:
//
//	stateOpen ─[1s 後]──▶ stateBlink1 ──tick──┬─[Normal/Slow]──▶ stateOpen ─[PickInterval]──▶ (loop)
//	                                            └─[Double]────▶ stateBetween ──tick──▶ stateBlink2 ──tick──▶ stateOpen
//
// 単一 goroutine (game.Update) からのみ使用すること（スレッドセーフではない）。
package blink

import (
	"math/rand/v2"
	"time"
)

// 瞬きパラメータ（定数化で off-by-one 防止）。
const (
	blinkNormalMinMs = 80
	blinkNormalMaxMs = 150
	blinkSlowMinMs   = 200
	blinkSlowMaxMs   = 300
	doubleGapMinMs   = 50
	doubleGapMaxMs   = 100
)

// BlinkType は瞬きの種類。
type BlinkType int

const (
	BlinkNormal BlinkType = iota // 通常の瞬き
	BlinkDouble                  // 二度瞬き
	BlinkSlow                    // ゆっくり瞬き
)

// blinkState は Scheduler の内部状態。
type blinkState int

const (
	stateOpen   blinkState = iota // 目を開いている
	stateBlink1                   // 1 回目の瞬き中
	stateBetween                  // 二度瞬きの中間（開いている）
	stateBlink2                   // 2 回目の瞬き中
)

// Scheduler は自動まばたきを制御する状態機械。
type Scheduler struct {
	rng *rand.Rand

	state             blinkState
	nextStateChangeAt time.Time
	currentBlinkType  BlinkType
}

// New は時間ベースの seed で新しい Scheduler を作成する。
func New() *Scheduler {
	return NewWithSeed(time.Now().UnixNano())
}

// NewWithSeed はテスト用に特定の seed で Scheduler を作成する。
// 第 2 seed は固定 (golden ratio 由来) で再現性確保。
func NewWithSeed(seed int64) *Scheduler {
	return &Scheduler{
		rng:              rand.New(rand.NewPCG(uint64(seed), 0x9E3779B97F4A7C15)),
		state:            stateOpen,
		nextStateChangeAt: time.Now().Add(1 * time.Second), // 最初の瞬きは 1s 後から
	}
}

// Update は現在時刻を受け取り、目を閉じているべきかどうかを返す。
// 内部状態を現在時刻まで進め、次の状態変化時刻も更新する。
func (s *Scheduler) Update(now time.Time) bool {
	if !now.Before(s.nextStateChangeAt) {
		s.advance(now)
	}
	return s.state == stateBlink1 || s.state == stateBlink2
}

func (s *Scheduler) advance(now time.Time) {
	switch s.state {
	case stateOpen:
		// 目を開いている状態 → 瞬き開始
		s.currentBlinkType = s.PickBlinkType()
		s.state = stateBlink1
		s.nextStateChangeAt = now.Add(s.blinkDuration())
	case stateBlink1:
		// 1 回目の瞬き終了
		if s.currentBlinkType == BlinkDouble {
			// 二度瞬き: 中間（開いた）状態へ
			gap := time.Duration(doubleGapMinMs+s.rng.IntN(doubleGapMaxMs-doubleGapMinMs+1)) * time.Millisecond
			s.state = stateBetween
			s.nextStateChangeAt = now.Add(gap)
		} else {
			// 通常 or ゆっくり: 瞬き終了、次の瞬きまで待機
			s.state = stateOpen
			s.nextStateChangeAt = now.Add(s.PickInterval())
		}
	case stateBetween:
		// 二度瞬きの中間終了 → 2 回目の瞬き開始
		s.state = stateBlink2
		s.nextStateChangeAt = now.Add(s.blinkDuration())
	case stateBlink2:
		// 2 回目の瞬き終了 → 次の瞬きまで待機
		s.state = stateOpen
		s.nextStateChangeAt = now.Add(s.PickInterval())
	}
}

// blinkDuration は現在の BlinkType に応じた瞬き継続時間を返す。
//   - BlinkSlow: 200-300ms
//   - Normal / Double: 80-150ms
func (s *Scheduler) blinkDuration() time.Duration {
	if s.currentBlinkType == BlinkSlow {
		return time.Duration(blinkSlowMinMs+s.rng.IntN(blinkSlowMaxMs-blinkSlowMinMs+1)) * time.Millisecond
	}
	// Normal / Double
	return time.Duration(blinkNormalMinMs+s.rng.IntN(blinkNormalMaxMs-blinkNormalMinMs+1)) * time.Millisecond
}

// PickBlinkType は 22/6/72 の分布で瞬きタイプを選ぶ。
func (s *Scheduler) PickBlinkType() BlinkType {
	r := s.rng.Float64()
	if r < 0.06 {
		return BlinkSlow
	}
	if r < 0.06+0.22 {
		return BlinkDouble
	}
	return BlinkNormal
}

// PickInterval は次の瞬きまでの interval を返す。
// 分布:
//   - r < 0.70: 1.8-4.5s (通常)
//   - r < 0.94: 0.7-1.5s (たまに短い)
//   - else:     4.5-9s   (ぼーっと長い)
//
// 境界値 r=0.70 は短い側、r=0.94 は長い側に分類される (measure zero 確率)。
func (s *Scheduler) PickInterval() time.Duration {
	r := s.rng.Float64()
	switch {
	case r < 0.70:
		return time.Duration(float64(time.Second) * (1.8 + s.rng.Float64()*2.7))
	case r < 0.94:
		return time.Duration(float64(time.Second) * (0.7 + s.rng.Float64()*0.8))
	default:
		return time.Duration(float64(time.Second) * (4.5 + s.rng.Float64()*4.5))
	}
}

// CurrentBlinkType は直近に開始された瞬きのタイプを返す。
// New() 直後は BlinkNormal (ゼロ値)。
// advance(stateOpen) で次の瞬きタイプが選択されるまで、前の瞬きタイプを保持する。
// デバッグ・ログ用途を想定。
func (s *Scheduler) CurrentBlinkType() BlinkType {
	return s.currentBlinkType
}
