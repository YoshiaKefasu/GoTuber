// Package blink は自動まばたきを制御する。
//
// 瞬きタイプ分布: 22% 二度瞬き / 6% ゆっくり / 72% 通常
// 瞬き間隔範囲: 1.8〜4.5s (70%, 通常) / 0.7〜1.5s (24%, たまに短く) / 4.5〜9s (6%, ぼーっと長く)
// 瞬き継続時間: 通常 80〜150ms / ゆっくり 200〜300ms
// 二度瞬き: 1 回目 80〜150ms → 50〜100ms 開く → 2 回目 80〜150ms
//
// 単一 goroutine (game.Update) からのみ使用すること（スレッドセーフではない）。
package blink

import (
	"math/rand"
	"time"
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
func NewWithSeed(seed int64) *Scheduler {
	return &Scheduler{
		rng:              rand.New(rand.NewSource(seed)),
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
			s.state = stateBetween
			s.nextStateChangeAt = now.Add(time.Duration(50+s.rng.Intn(50)) * time.Millisecond)
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

func (s *Scheduler) blinkDuration() time.Duration {
	if s.currentBlinkType == BlinkSlow {
		return time.Duration(200+s.rng.Intn(100)) * time.Millisecond
	}
	// Normal / Double: 80-150ms
	return time.Duration(80+s.rng.Intn(70)) * time.Millisecond
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
// 分布: 70% で 1.8-4.5s、24% で 0.7-1.5s、6% で 4.5-9s
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

// BlinkType は現在の瞬きタイプを返す（stateOpen の場合は BlinkNormal、デバッグ用）。
func (s *Scheduler) CurrentBlinkType() BlinkType {
	return s.currentBlinkType
}
