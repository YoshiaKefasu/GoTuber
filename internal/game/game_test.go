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
// 真理値表は passthroughDesired 関数の godoc を参照。
func TestGame_PassthroughDesired(t *testing.T) {
	tests := []struct {
		name         string
		panelVisible bool
		uiHidden     bool
		want         bool
	}{
		{"panel only (UI clickable)", true, false, false},
		{"panel + hidden (UI all hidden)", true, true, true},
		{"hidden only (no panel anyway)", false, true, true},
		{"nothing (default startup)", false, false, true},
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
