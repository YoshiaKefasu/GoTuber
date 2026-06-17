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
	if s.AudioRMS != 0 {
		t.Errorf("expected AudioRMS=0, got %v", s.AudioRMS)
	}
	if s.AudioEnvelope != 0 {
		t.Errorf("expected AudioEnvelope=0, got %v", s.AudioEnvelope)
	}
	if s.AudioMouthState != 0 {
		t.Errorf("expected AudioMouthState=0, got %v", s.AudioMouthState)
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

// TestNewPanel は NewPanel が panic せず Panel を返すことを確認。
// ebitenui の widget ツリー構築は ebiten context 不要 (NewPanel 時点では)。
func TestNewPanel(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("NewPanel panicked: %v", r)
		}
	}()
	face := LoadFontFace(16)
	state := NewState()
	panel := NewPanel(face, state, true)
	if panel == nil {
		t.Errorf("expected non-nil panel")
	}
}

// TestNewPanel_NoAudio は audioEnabled=false でも Panel が構築できることを確認。
func TestNewPanel_NoAudio(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("NewPanel panicked: %v", r)
		}
	}()
	face := LoadFontFace(16)
	state := NewState()
	panel := NewPanel(face, state, false)
	if panel == nil {
		t.Errorf("expected non-nil panel even without audio")
	}
}

// TestClampInt はクランプロジックの境界確認。
func TestClampInt(t *testing.T) {
	tests := []struct {
		name           string
		v, lo, hi, exp int
	}{
		{"in range", 50, 5, 100, 50},
		{"below lo", 0, 5, 100, 5},
		{"above hi", 150, 5, 100, 100},
		{"at lo", 5, 5, 100, 5},
		{"at hi", 100, 5, 100, 100},
		{"negative", -10, 0, 10, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clampInt(tt.v, tt.lo, tt.hi)
			if got != tt.exp {
				t.Errorf("clampInt(%d, %d, %d) = %d, want %d", tt.v, tt.lo, tt.hi, got, tt.exp)
			}
		})
	}
}

// TestSliderConstants はスライダー定数の値域確認。
func TestSliderConstants(t *testing.T) {
	if sliderMin >= sliderMax {
		t.Errorf("sliderMin (%d) should be < sliderMax (%d)", sliderMin, sliderMax)
	}
	if sliderMin < 1 || sliderMin > 10 {
		t.Errorf("sliderMin (%d) should be in 1-10 (representing 0.01-0.10 of MouseResponsiveness)", sliderMin)
	}
	if sliderMax != 100 {
		t.Errorf("sliderMax (%d) should be 100 (representing 1.0)", sliderMax)
	}
}

func TestAudioDebugLabel(t *testing.T) {
	s := NewState()
	s.AudioRMS = 0.12345
	s.AudioEnvelope = 0.06789
	s.AudioMouthState = 1
	got := audioDebugLabel(s)
	want := "Audio RMS: 0.1235 | Envelope: 0.0679 | Mouth: half"
	if got != want {
		t.Errorf("audioDebugLabel() = %q, want %q", got, want)
	}
}

func TestMouthStateLabel(t *testing.T) {
	tests := []struct {
		state int
		want  string
	}{
		{0, "closed"},
		{1, "half"},
		{2, "open"},
		{99, "closed"},
	}
	for _, tt := range tests {
		if got := mouthStateLabel(tt.state); got != tt.want {
			t.Errorf("mouthStateLabel(%d) = %q, want %q", tt.state, got, tt.want)
		}
	}
}

// TestPanel_SetUIHidden は SetUIHidden フラグの動作確認 (Phase 1.13b)。
// Ctrl+Shift+H トグル時に Game.Update() から呼ばれる想定。
func TestPanel_SetUIHidden(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("SetUIHidden panicked: %v", r)
		}
	}()
	face := LoadFontFace(16)
	state := NewState()
	panel := NewPanel(face, state, true)
	if panel.uiHidden {
		t.Error("expected initial uiHidden=false")
	}
	panel.SetUIHidden(true)
	if !panel.uiHidden {
		t.Error("expected uiHidden=true after SetUIHidden(true)")
	}
	panel.SetUIHidden(false)
	if panel.uiHidden {
		t.Error("expected uiHidden=false after SetUIHidden(false)")
	}
}

// TestPanel_Draw_SkipsWhenUIHidden は uiHidden=true で Draw() が即 return することを確認 (Phase 1.13b)。
// nil image を渡しても uiHidden なら panic しない。
// (実画面描画は ebiten context が必要なため、ここでは no-op であることだけ検証)
func TestPanel_Draw_SkipsWhenUIHidden(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Draw with uiHidden=true panicked: %v", r)
		}
	}()
	face := LoadFontFace(16)
	state := NewState()
	panel := NewPanel(face, state, true)
	panel.SetUIHidden(true)
	// nil image を渡しても uiHidden なら即 return するはず
	panel.Draw(nil)
}
