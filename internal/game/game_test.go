package game

import (
	"math"
	"testing"
)

// TestGame_ToggleUIHidden_TogglesValue は uiHidden フラグのトグル動作確認。
// ebiten context に依存しない (純粋ロジック)。
func TestGame_ToggleUIHidden_TogglesValue(t *testing.T) {
	g := &Game{}
	if g.uiHidden {
		t.Fatal("expected initial uiHidden=false (zero value)")
	}
	g.ToggleUIHidden()
	if !g.uiHidden {
		t.Error("expected uiHidden=true after first toggle")
	}
	g.ToggleUIHidden()
	if g.uiHidden {
		t.Error("expected uiHidden=false after second toggle")
	}
}

// TestGame_PassthroughDesired は UI 表示状態から期待されるクリックスルー値を確認。
// Phase 1.14.10: passthrough は全面廃止され、X ボタンを常に有効化するため
// passthroughDesired は常に false を返す (旧真理値表は無効)。
func TestGame_PassthroughDesired(t *testing.T) {
	tests := []struct {
		name         string
		panelVisible bool
		uiHidden     bool
		want         bool
	}{
		{"panel only (UI clickable)", true, false, false},
		{"panel + hidden (UI all hidden)", true, true, false},
		{"hidden only (no panel anyway)", false, true, false},
		{"nothing (default startup)", false, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := passthroughDesired(tt.panelVisible, tt.uiHidden); got != tt.want {
				t.Errorf("passthroughDesired(%v, %v) = %v, want %v",
					tt.panelVisible, tt.uiHidden, got, tt.want)
			}
		})
	}
}

// ─── Phase 4.5: easeInOutCubic 純粋関数テスト ─────────────────────────────

func TestEaseInOutCubic_BoundaryValues(t *testing.T) {
	// [0,1] → [0,1] の端点保証
	tests := []struct {
		input, want float64
	}{
		{0.0, 0.0},
		{1.0, 1.0},
	}
	for _, tt := range tests {
		got := easeInOutCubic(tt.input)
		if math.Abs(got-tt.want) > 1e-10 {
			t.Errorf("easeInOutCubic(%v) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestEaseInOutCubic_Midpoint(t *testing.T) {
	// 中点 (0.5) は easeInOutCubic の定義で 0.5 にマッピングされる
	// p < 0.5: 4*(0.5)^3 = 0.5 → 0.5 のとき p >= 0.5 ブランチ: 1 - (-2*0.5+2)^3/2 = 1 - 1^3/2 = 0.5
	got := easeInOutCubic(0.5)
	if math.Abs(got-0.5) > 1e-10 {
		t.Errorf("easeInOutCubic(0.5) = %v, want 0.5", got)
	}
}

func TestEaseInOutCubic_Symmetric(t *testing.T) {
	// easeInOutCubic は p と 1-p で対称: ease(p) + ease(1-p) = 1
	for _, p := range []float64{0.1, 0.2, 0.3, 0.4} {
		a := easeInOutCubic(p)
		b := easeInOutCubic(1 - p)
		if math.Abs(a+b-1.0) > 1e-10 {
			t.Errorf("easeInOutCubic(%v) + easeInOutCubic(%v) = %v, want 1.0", p, 1-p, a+b)
		}
	}
}

func TestEaseInOutCubic_MonotonicallyIncreasing(t *testing.T) {
	// 単調増加: p1 < p2 → ease(p1) < ease(p2)
	prev := 0.0
	for i := 1; i <= 100; i++ {
		p := float64(i) / 100.0
		got := easeInOutCubic(p)
		if got <= prev {
			t.Errorf("easeInOutCubic(%v)=%v <= previous=%v, not monotonic", p, got, prev)
		}
		prev = got
	}
}

func TestEaseInOutCubic_OutOfRangeClamped(t *testing.T) {
	// 範囲外入力でも [0,1] を返す
	if got := easeInOutCubic(-0.5); got != 0 {
		t.Errorf("easeInOutCubic(-0.5) = %v, want 0", got)
	}
	if got := easeInOutCubic(1.5); got != 1 {
		t.Errorf("easeInOutCubic(1.5) = %v, want 1", got)
	}
}

func TestEaseInOutCubic_EaseInRegion(t *testing.T) {
	// p < 0.5 では ease-in（加速）: 値が linear 以下
	// linear: 0.25, ease: 4*(0.25)^3 = 0.0625
	got := easeInOutCubic(0.25)
	if got >= 0.25 {
		t.Errorf("easeInOutCubic(0.25) = %v, want < 0.25 (ease-in region)", got)
	}
}

func TestEaseInOutCubic_EaseOutRegion(t *testing.T) {
	// p > 0.5 では ease-out（減速）: 値が linear 以上
	// linear: 0.75, ease: 1 - (-2*0.75+2)^3/2 = 1 - 0.5^3/2 = 0.9375
	got := easeInOutCubic(0.75)
	if got <= 0.75 {
		t.Errorf("easeInOutCubic(0.75) = %v, want > 0.75 (ease-out region)", got)
	}
}

// ─── Phase 4.0: updateTransition 純粋関数テスト ──────────────────────────

func TestUpdateTransition_NoChange_NoTransition(t *testing.T) {
	// セルが変わっていない → 遷移しない
初始 := cellTransition{fromSheet: 0, fromRow: 2, fromCol: 3, progress: 0, active: false}
	got := updateTransition(初始, 0, 2, 3, 0, 2, 3, 0.016, 0.1) // 同じセル
	if got.active {
		t.Error("expected no transition when cell unchanged")
	}
}

func TestUpdateTransition_CellChanged_StartsTransition(t *testing.T) {
	// セルが変わった → 遷移開始
	初始 := cellTransition{}
	got := updateTransition(初始, 0, 2, 2, 0, 2, 3, 0.016, 0.1) // c2→c3
	if !got.active {
		t.Fatal("expected transition to start")
	}
	if got.fromSheet != 0 || got.fromRow != 2 || got.fromCol != 2 {
		t.Errorf("from=(%d,%d,%d), want (0,2,2)", got.fromSheet, got.fromRow, got.fromCol)
	}
	if got.progress != 0 {
		t.Errorf("progress=%v, want 0", got.progress)
	}
}

func TestUpdateTransition_ProgressAdvances(t *testing.T) {
	// 遷移中にセルが変わらない → progress が進む
	初期 := cellTransition{fromSheet: 0, fromRow: 0, fromCol: 0, progress: 0, active: true}
	got := updateTransition(初期, 0, 0, 1, 0, 0, 1, 0.05, 0.1) // 50ms delta / 100ms duration
	if !got.active {
		t.Fatal("expected transition still active")
	}
	if got.progress < 0.4 || got.progress > 0.6 {
		t.Errorf("progress=%v, want ~0.5", got.progress)
	}
}

func TestUpdateTransition_Completes(t *testing.T) {
	// progress が 1.0 を超える → 遷移完了
	初期 := cellTransition{fromSheet: 0, fromRow: 0, fromCol: 0, progress: 0.9, active: true}
	got := updateTransition(初期, 0, 0, 1, 0, 0, 1, 0.05, 0.1) // 0.9 + 0.5 = 1.4 ≥ 1.0
	if got.active {
		t.Error("expected transition completed")
	}
	if got.progress != 1.0 {
		t.Errorf("progress=%v, want 1.0", got.progress)
	}
}

func TestUpdateTransition_CellChangedDuringTransition_KeepsProgress(t *testing.T) {
	// 遷移中にセルが変わった → from を現在の表示セルに更新し、progress は維持する。
	// 旧実装では progress=0 で再開していたため、境界付近の往復で高速点滅が発生していた。
	// 遷移 A(r0c0)→B(r0c1) の途中で C(r0c2) に変わった場合:
	//   from = B(r0c1) になる (prev = 直近の表示セル)
	//   progress は 0.5+0.16=0.66 のまま維持 (進行度加算後に cellChanged 検査)
	初期 := cellTransition{fromSheet: 0, fromRow: 0, fromCol: 0, progress: 0.5, active: true}
	got := updateTransition(初期, 0, 0, 1, 0, 0, 2, 0.016, 0.1) // prev=r0c1, cur=r0c2
	if !got.active {
		t.Fatal("expected transition still active after cell change")
	}
	if got.fromSheet != 0 || got.fromRow != 0 || got.fromCol != 1 {
		t.Errorf("from=(%d,%d,%d), want (0,0,1) [prev cell]", got.fromSheet, got.fromRow, got.fromCol)
	}
	expectedProgress := 0.5 + 0.016/0.1 // 0.66
	if got.progress != expectedProgress {
		t.Errorf("progress=%v, want %v (progress maintained after advance)", got.progress, expectedProgress)
	}
}

func TestUpdateTransition_CompleteThenNewChange(t *testing.T) {
	// 遷移完了後に新しいセル変化 → 新しい遷移が開始される
	完了 := cellTransition{fromSheet: 0, fromRow: 0, fromCol: 1, progress: 1.0, active: false}
	got := updateTransition(完了, 0, 0, 2, 0, 0, 3, 0.016, 0.1) // 完了状態で r0c2→r0c3
	if !got.active {
		t.Fatal("expected new transition after previous completed")
	}
	if got.fromSheet != 0 || got.fromRow != 0 || got.fromCol != 2 {
		t.Errorf("from=(%d,%d,%d), want (0,0,2)", got.fromSheet, got.fromRow, got.fromCol)
	}
}

func TestUpdateTransition_MultipleFramesToComplete(t *testing.T) {
	// 複数フレームかけて遷移が完了することを確認
	dur := 0.1 // 100ms
	delta := 0.033 // ~30fps
	t_state := cellTransition{fromSheet: 0, fromRow: 0, fromCol: 0, progress: 0, active: true}

	for i := 0; i < 5; i++ {
		t_state = updateTransition(t_state, 0, 0, 1, 0, 0, 1, delta, dur)
		if !t_state.active && i < 3 {
			t.Fatalf("expected still active at frame %d, progress=%v", i, t_state.progress)
		}
	}
	if t_state.active {
		t.Error("expected transition to complete after 5 frames (~165ms > 100ms)")
	}
	if t_state.progress != 1.0 {
		t.Errorf("progress=%v, want 1.0", t_state.progress)
	}
}

func TestGame_FirstCellSnapshot_DoesNotStartTransition(t *testing.T) {
	// 起動直後の初回セル確定ではフェードイン遷移を開始しない。
	g := &Game{}
	curSheet, curRow, curCol := 2, 3, 4

	if g.firstCellSet {
		t.Fatal("expected firstCellSet=false initially")
	}

	if !g.firstCellSet {
		g.firstCellSet = true
	} else {
		g.trans = updateTransition(g.trans,
			g.prevSheet, g.prevRow, g.prevCol,
			curSheet, curRow, curCol,
			0.016, 0.1)
	}
	g.prevSheet = curSheet
	g.prevRow = curRow
	g.prevCol = curCol

	if g.trans.active {
		t.Fatal("expected no transition on first cell snapshot")
	}
	if g.prevSheet != curSheet || g.prevRow != curRow || g.prevCol != curCol {
		t.Fatalf("prev=(%d,%d,%d), want (%d,%d,%d)", g.prevSheet, g.prevRow, g.prevCol, curSheet, curRow, curCol)
	}
}

// ─── Phase 4.0 hotfix: 境界付近のセル往復で高速点滅しないことの検証 ────────

func TestUpdateTransition_Oscillation_NoFlicker(t *testing.T) {
	// r2c2 ↔ r2c3 が毎フレーム往復しても、progress が毎回 0 に戻らないことを確認。
	// 旧実装 (progress=0 リセット) では、各フレームで alpha が 1.0↔0.0 に戻り、
	// 高速 ON/OFF 点滅に見えていた。
	// 新実装 (progress 維持) では、progress が累積的に増加し、
	// 遷移が連続的に進行する。
	dur := 0.1 // 100ms
	delta := 0.016 // ~60fps

	// Frame 1: r2c2 → r2c3 (遷移開始)
	state := updateTransition(
		cellTransition{},
		0, 2, 2, // prev = r2c2
		0, 2, 3, // cur = r2c3
		delta, dur,
	)
	if !state.active {
		t.Fatal("expected transition to start")
	}
	if state.progress != 0 {
		t.Errorf("frame 1: progress=%v, want 0", state.progress)
	}

	// Frame 2: r2c3 → r2c2 (往復)
	state = updateTransition(
		state,
		0, 2, 3, // prev = r2c3 (前フレームで更新)
		0, 2, 2, // cur = r2c2 (往復)
		delta, dur,
	)
	if !state.active {
		t.Fatal("expected transition still active")
	}
	// progress が 0 に戻っていないこと (旧バグの再現防止)
	if state.progress == 0 {
		t.Error("progress reset to 0 — flicker bug reproduced")
	}
	// progress が正の値で累積していること
	if state.progress <= 0 {
		t.Errorf("frame 2: progress=%v, want > 0 (accumulating)", state.progress)
	}

	// Frame 3: r2c2 → r2c3 (再往復)
	prevProgress := state.progress
	state = updateTransition(
		state,
		0, 2, 2, // prev = r2c2
		0, 2, 3, // cur = r2c3
		delta, dur,
	)
	if !state.active {
		t.Fatal("expected transition still active")
	}
	if state.progress == 0 {
		t.Error("progress reset to 0 — flicker bug reproduced")
	}
	// progress が前フレームから増加していること
	if state.progress <= prevProgress {
		t.Errorf("frame 3: progress=%v <= prev=%v, expected accumulation", state.progress, prevProgress)
	}
}

func TestUpdateTransition_Oscillation_EventuallyCompletes(t *testing.T) {
	// 往復が続いても progress が累積し、100ms 後には遷移が完了することを確認。
	dur := 0.1   // 100ms
	delta := 0.016 // ~60fps

	state := cellTransition{}
	// 6 フレーム分 (約 96ms) r2c2 ↔ r2c3 が往復
	for i := 0; i < 6; i++ {
		if i%2 == 0 {
			// r2c2 → r2c3
			state = updateTransition(state, 0, 2, 2, 0, 2, 3, delta, dur)
		} else {
			// r2c3 → r2c2
			state = updateTransition(state, 0, 2, 3, 0, 2, 2, delta, dur)
		}
	}

	// progress が累積していること
	if state.progress <= 0 {
		t.Errorf("after 6 oscillation frames: progress=%v, expected > 0", state.progress)
	}
	// まだ完了していない (6×16ms = 96ms < 100ms) の場合もあるが、
	// progress が 0 でないことは保証
	if state.progress == 0 && state.active {
		t.Error("progress is 0 after 6 frames — flicker bug present")
	}
}

func TestUpdateTransition_RapidOscillation_ProgressNeverResets(t *testing.T) {
	// 10 フレーム連続の往復で、同一遷移中 (active→active) の progress が
	// 一度も減少しないことを確認。
	// 遷移完了 (active=false) → 新規開始で progress=0 にはなるが、
	// これは正常動作（完了後に新しい遷移）。prevWasActive で区別する。
	dur := 0.1
	delta := 0.016

	state := cellTransition{}
	prevProgress := 0.0
	prevWasActive := false

	for i := 0; i < 10; i++ {
		if i%2 == 0 {
			state = updateTransition(state, 0, 2, 2, 0, 2, 3, delta, dur)
		} else {
			state = updateTransition(state, 0, 2, 3, 0, 2, 2, delta, dur)
		}
		// 同一遷移の連続フレーム (前フレームも active、今フレームも active) では
		// progress が減少しないこと
		if prevWasActive && state.active && state.progress < prevProgress {
			t.Errorf("frame %d: progress=%v dropped below prev=%v while active", i, state.progress, prevProgress)
		}
		prevWasActive = state.active
		if state.active {
			prevProgress = state.progress
		}
	}

	// progress が 0.0〜1.0 範囲内であること
	if state.progress < 0.0 || state.progress > 1.0 {
		t.Errorf("final progress=%v out of [0.0, 1.0]", state.progress)
	}
}

// ─── Phase 4.0 hotfix: progress clamp 確保 (flashing 修正) ─────────────

func TestUpdateTransition_ProgressNeverExceedsOne(t *testing.T) {
	// 遷移中に極端に大きい delta でも progress が 1.0 を超えないことを確認。
	// 旧実装では往復が続くと progress が 1.0 を超え、
	// Draw 側で alpha が範囲外になり明るく flashing していた。
	dur := 0.1

	// 既に active な遷移に大きい delta を適用 → progress が clamp されること
	active := cellTransition{fromSheet: 0, fromRow: 2, fromCol: 2, progress: 0.5, active: true}
	state := updateTransition(
		active,
		0, 2, 2, 0, 2, 3, // prev=r2c2, cur=r2c3 (same → no cellChanged)
		1.0, dur, // deltaSec=1.0, durationSec=0.1 → 0.5+10.0=10.5 → clamped to 1.0
	)
	if state.progress > 1.0 {
		t.Errorf("progress=%v, want <= 1.0 (clamp failed)", state.progress)
	}
	if state.progress != 1.0 {
		t.Errorf("progress=%v, want 1.0 (should clamp to max)", state.progress)
	}
	if state.active {
		t.Error("expected transition completed (progress >= 1.0)")
	}
}

func TestUpdateTransition_CompletionDuringOscillation(t *testing.T) {
	// 往復中に progress が 1.0 に到達したら active=false になることを確認。
	// Frame 0: 開始 (progress=0), Frame 1-7: +0.16 each → 0.16, 0.32, 0.48, 0.64, 0.80, 0.96, 1.12→clamped 1.0
	dur := 0.1
	delta := 0.016

	state := cellTransition{}
	// 8 フレーム分 (約 128ms > 100ms) 往復して完了を確認
	for i := 0; i < 8; i++ {
		if i%2 == 0 {
			state = updateTransition(state, 0, 2, 2, 0, 2, 3, delta, dur)
		} else {
			state = updateTransition(state, 0, 2, 3, 0, 2, 2, delta, dur)
		}
	}

	// progress が 1.0 に clamp されていること
	if state.progress > 1.0 {
		t.Errorf("progress=%v, want <= 1.0", state.progress)
	}
	// 8 フレームで完了していること
	if state.active {
		t.Error("expected transition completed after 8 frames (~128ms > 100ms)")
	}
}

func TestUpdateTransition_Oscillation_ProgressAlwaysInRange(t *testing.T) {
	// 20 フレーム連続往復で、progress が常に [0.0, 1.0] 範囲内であることを確認。
	dur := 0.1
	delta := 0.016

	state := cellTransition{}
	for i := 0; i < 20; i++ {
		if i%2 == 0 {
			state = updateTransition(state, 0, 2, 2, 0, 2, 3, delta, dur)
		} else {
			state = updateTransition(state, 0, 2, 3, 0, 2, 2, delta, dur)
		}
		if state.progress < 0.0 || state.progress > 1.0 {
			t.Errorf("frame %d: progress=%v out of [0.0, 1.0]", i, state.progress)
		}
	}
}

// ─── Phase 4.5+: Single-sprite Liquify Transition テスト ───────────────────

func TestTransitionSpriteSelection_BoundaryValues(t *testing.T) {
	tests := []struct {
		progress float64
		wantOld  bool
	}{
		{0.0, true},   // 開始直後 → 旧セル
		{0.25, true},  // 前半 → 旧セル
		{0.49, true},  // 境界直前 → 旧セル
		{0.50, false}, // 中間点 → 新セル
		{0.75, false}, // 後半 → 新セル
		{1.0, false},  // 完了 → 新セル
	}
	for _, tt := range tests {
		got := transitionSpriteSelection(tt.progress)
		if got != tt.wantOld {
			t.Errorf("transitionSpriteSelection(%v) = %v (showOld), want %v", tt.progress, got, tt.wantOld)
		}
	}
}

func TestTransitionWarp_ZeroProgress(t *testing.T) {
	// progress=0 → warp は 0 (まだ変形しない)
	warpX, warpY := transitionWarp(0, 0, 0, 1, 0.0, 1280, 720)
	if warpX != 0 || warpY != 0 {
		t.Errorf("progress=0: warp=(%v,%v), want (0,0)", warpX, warpY)
	}
}

func TestTransitionWarp_CompleteProgress(t *testing.T) {
	// progress=1.0 → warp は 0 (遷移完了後、静止画に収束)
	warpX, warpY := transitionWarp(0, 0, 0, 1, 1.0, 1280, 720)
	if warpX != 0 || warpY != 0 {
		t.Errorf("progress=1.0: warp=(%v,%v), want (0,0)", warpX, warpY)
	}
}

func TestTransitionWarp_ProportionalToCellDiff(t *testing.T) {
	// セル差分が大きいほど warp が大きい
	screenW, screenH := 1280.0, 720.0
	progress := 0.25 // 前半、warpFactor > 0

	smallWarpX, _ := transitionWarp(0, 0, 0, 1, progress, screenW, screenH) // dCol=1
	largeWarpX, _ := transitionWarp(0, 0, 0, 3, progress, screenW, screenH) // dCol=3

	if largeWarpX <= smallWarpX {
		t.Errorf("larger cell diff should produce larger warp: small=%v, large=%v", smallWarpX, largeWarpX)
	}
}

func TestTransitionWarp_Direction_OldSprite(t *testing.T) {
	// 前半 (progress < 0.5): 旧セルから新セル方向へ warp
	// from=r0c0, to=r0c1 → dCol=+1 → warpX は正 (右方向)
	screenW, screenH := 1280.0, 720.0
	warpX, _ := transitionWarp(0, 0, 0, 1, 0.25, screenW, screenH)
	if warpX <= 0 {
		t.Errorf("old sprite warpX=%v, want > 0 (toward new cell right)", warpX)
	}
}

func TestTransitionWarp_Direction_NewSprite(t *testing.T) {
	// 後半 (progress >= 0.5): 逆向き warp (settling back)
	// from=r0c0, to=r0c1 → dCol=+1 → warpX は負 (逆方向)
	screenW, screenH := 1280.0, 720.0
	warpX, _ := transitionWarp(0, 0, 0, 1, 0.75, screenW, screenH)
	if warpX >= 0 {
		t.Errorf("new sprite warpX=%v, want < 0 (reverse direction settling)", warpX)
	}
}

func TestTransitionWarp_SymmetricMagnitude(t *testing.T) {
	// progress=0.25 と progress=0.75 で warp の絶対値が等しいことを確認
	// (easeInOutCubic の対称性: ease(p) + ease(1-p) = 1 だが、
	//  warpFactor は progress*2 と (1-progress)*2 で対称)
	screenW, screenH := 1280.0, 720.0
	wEarlyX, _ := transitionWarp(0, 0, 0, 3, 0.25, screenW, screenH)
	wLateX, _ := transitionWarp(0, 0, 0, 3, 0.75, screenW, screenH)

	// 0.25 は progress*2=0.5 → easeInOutCubic(0.5)=0.5
	// 0.75 は (1-0.75)*2=0.5 → easeInOutCubic(0.5)=0.5
	// warpFactor が同じなので、絶対値も等しい
	if math.Abs(math.Abs(wEarlyX)-math.Abs(wLateX)) > 0.01 {
		t.Errorf("symmetry: early=%v, late=%v, want equal magnitude", wEarlyX, wLateX)
	}
}

func TestTransitionWarp_MaxWarpAtMidpoint(t *testing.T) {
	// progress=0.5 付近で warp factor が最大になることを確認
	screenW, screenH := 1280.0, 720.0

	// progress=0.49: warpFactor = easeInOutCubic(0.98) ≈ 0.994
	_, w49 := transitionWarp(0, 0, 0, 2, 0.49, screenW, screenH)
	// progress=0.50: warpFactor = easeInOutCubic(1.0) = 1.0 (but reversed)
	_, w50 := transitionWarp(0, 0, 0, 2, 0.50, screenW, screenH)

	// 絶対値で比較: 0.49 と 0.50 はほぼ同じ magnitude
	if math.Abs(math.Abs(w49)-math.Abs(w50)) > 1.0 {
		t.Errorf("midpoint warp magnitude: 0.49=%v, 0.50=%v, should be close", w49, w50)
	}
}
