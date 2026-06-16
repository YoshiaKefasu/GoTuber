package tweaks

import "testing"

// TestState_Defaults は NewState がデフォルト値で初期化されることを確認。
func TestState_Defaults(t *testing.T) {
	s := NewState()
	if s.MouseResponsiveness != 0.3 {
		t.Errorf("expected MouseResponsiveness=0.3, got %v", s.MouseResponsiveness)
	}
	if !s.BlinkEnabled {
		t.Errorf("expected BlinkEnabled=true, got false")
	}
	if !s.AudioEnabled {
		t.Errorf("expected AudioEnabled=true, got false")
	}
	if s.PanelVisible {
		t.Errorf("expected PanelVisible=false, got true")
	}
	if s.QuitRequested {
		t.Errorf("expected QuitRequested=false, got true")
	}
}

// TestState_QuitRequestedToggle は QuitRequested フラグの動作確認。
func TestState_QuitRequestedToggle(t *testing.T) {
	s := NewState()
	if s.QuitRequested {
		t.Errorf("initial: expected false, got true")
	}
	s.QuitRequested = true
	if !s.QuitRequested {
		t.Errorf("after set: expected true, got false")
	}
}

// TestState_PanelVisibleToggle は PanelVisible のトグル動作確認。
func TestState_PanelVisibleToggle(t *testing.T) {
	s := NewState()
	s.PanelVisible = true
	if !s.PanelVisible {
		t.Errorf("expected true, got false")
	}
	s.PanelVisible = !s.PanelVisible
	if s.PanelVisible {
		t.Errorf("expected false after toggle, got true")
	}
}

// TestLoadFontFace は LoadFontFace が panic せずに text.Face を返すことを確認。
func TestLoadFontFace(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("LoadFontFace panicked: %v", r)
		}
	}()
	face := LoadFontFace(16)
	if face == nil {
		t.Errorf("expected non-nil face")
	}
	if face.Size != 16 {
		t.Errorf("expected Size=16, got %v", face.Size)
	}
}
