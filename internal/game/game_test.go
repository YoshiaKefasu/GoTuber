package game

import "testing"

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

func TestUpdateTransition_CellChangedDuringTransition_Restarts(t *testing.T) {
	// 遷移中にセルが変わった → from を現在の遷移先に更新して再開
	// 遷移 A(r0c0)→B(r0c1) の途中で C(r0c2) に変わった場合:
	//   from = B(r0c1) になる (prev = 直近の遷移先)
	初期 := cellTransition{fromSheet: 0, fromRow: 0, fromCol: 0, progress: 0.5, active: true}
	got := updateTransition(初期, 0, 0, 1, 0, 0, 2, 0.016, 0.1) // prev=r0c1, cur=r0c2
	if !got.active {
		t.Fatal("expected transition still active after restart")
	}
	if got.fromSheet != 0 || got.fromRow != 0 || got.fromCol != 1 {
		t.Errorf("from=(%d,%d,%d), want (0,0,1) [prev cell]", got.fromSheet, got.fromRow, got.fromCol)
	}
	if got.progress != 0 {
		t.Errorf("progress=%v, want 0 (restart)", got.progress)
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
