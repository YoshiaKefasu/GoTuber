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
	if s.AudioSensitivity != 10.0 {
		t.Errorf("expected AudioSensitivity=10.0, got %v", s.AudioSensitivity)
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
	// Phase 1.14.14: 追加された noise gate debug 値もゼロ値初期化される
	if s.AudioNoiseFloor != 0 {
		t.Errorf("expected AudioNoiseFloor=0, got %v", s.AudioNoiseFloor)
	}
	if s.AudioGatedRMS != 0 {
		t.Errorf("expected AudioGatedRMS=0, got %v", s.AudioGatedRMS)
	}
	if s.AudioGateOpen {
		t.Errorf("expected AudioGateOpen=false, got true")
	}
	// Phase 2.10.8: CameraEnabled はデフォルト true
	if !s.CameraEnabled {
		t.Errorf("expected CameraEnabled=true (Phase 2.10.8 default), got false")
	}
	// Phase 4.3: Morph フィールドのデフォルト値
	if !s.MorphEnabled {
		t.Errorf("expected MorphEnabled=true (Phase 4.3 default), got false")
	}
	if s.MorphStrength != 4.0 {
		t.Errorf("expected MorphStrength=4.0 (Phase 4.3 hotfix default), got %v", s.MorphStrength)
	}
	if s.TransitionDuration != 120.0 {
		t.Errorf("expected TransitionDuration=120.0 (Phase 4.5 tuning default), got %v", s.TransitionDuration)
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
	panel := NewPanel(face, state, true, "")
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
	panel := NewPanel(face, state, false, "")
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
	if sensitivitySliderMin != 10 {
		t.Errorf("sensitivitySliderMin (%d) should be 10 (representing 1.0x)", sensitivitySliderMin)
	}
	if sensitivitySliderMax != 200 {
		t.Errorf("sensitivitySliderMax (%d) should be 200 (representing 20.0x)", sensitivitySliderMax)
	}
	// Phase 4.3: Morph slider 定数
	if morphStrengthSliderMin != 0 {
		t.Errorf("morphStrengthSliderMin (%d) should be 0", morphStrengthSliderMin)
	}
	if morphStrengthSliderMax != 8 {
		t.Errorf("morphStrengthSliderMax (%d) should be 8 (Phase 4.3 hotfix)", morphStrengthSliderMax)
	}
	if transitionDurationSliderMin != 50 {
		t.Errorf("transitionDurationSliderMin (%d) should be 50", transitionDurationSliderMin)
	}
	if transitionDurationSliderMax != 250 {
		t.Errorf("transitionDurationSliderMax (%d) should be 250 (Phase 4.5 tuning)", transitionDurationSliderMax)
	}
}

func TestMicSensitivityLabelText(t *testing.T) {
	s := NewState()
	s.AudioSensitivity = 10.0
	if got, want := micSensitivityLabelText(s), "Mic Sensitivity: 10.0x"; got != want {
		t.Errorf("micSensitivityLabelText() = %q, want %q", got, want)
	}
	s.AudioSensitivity = 7.5
	if got, want := micSensitivityLabelText(s), "Mic Sensitivity: 7.5x"; got != want {
		t.Errorf("micSensitivityLabelText() = %q, want %q", got, want)
	}
}

// TestMorphStrengthLabelFmt は Morph Strength ラベルのフォーマット確認 (Phase 4.3)。
func TestMorphStrengthLabelFmt(t *testing.T) {
	s := NewState()
	s.MorphStrength = 8.0
	if got, want := morphStrengthLabelFmt(s), "Morph Strength: 8.0px"; got != want {
		t.Errorf("morphStrengthLabelFmt() = %q, want %q", got, want)
	}
	s.MorphStrength = 0.0
	if got, want := morphStrengthLabelFmt(s), "Morph Strength: 0.0px"; got != want {
		t.Errorf("morphStrengthLabelFmt() = %q, want %q", got, want)
	}
	s.MorphStrength = 12.5
	if got, want := morphStrengthLabelFmt(s), "Morph Strength: 12.5px"; got != want {
		t.Errorf("morphStrengthLabelFmt() = %q, want %q", got, want)
	}
}

// TestAudioDebugLabel1 は 1 行目 (raw RMS / Floor / Gate) のフォーマット確認。
// Phase 1.14.14: noise gate debug 値 (Floor / GateOpen) を表示するようになった。
func TestAudioDebugLabel1(t *testing.T) {
	s := NewState()
	s.AudioRMS = 0.12345
	s.AudioNoiseFloor = 0.00231
	s.AudioGateOpen = true
	got := audioDebugLabel1(s)
	want := "Audio RMS: 0.1235 | Floor: 0.0023 | Gate: open"
	if got != want {
		t.Errorf("audioDebugLabel1() = %q, want %q", got, want)
	}
}

// TestAudioDebugLabel2 は 2 行目 (Gated / Envelope / Mouth) のフォーマット確認。
// Phase 1.14.14: 2 行表示に拡張。gate 通過 + gain 後の値を表示。
func TestAudioDebugLabel2(t *testing.T) {
	s := NewState()
	s.AudioGatedRMS = 0.07890
	s.AudioEnvelope = 0.04567
	s.AudioMouthState = 1
	got := audioDebugLabel2(s)
	want := "Gated: 0.0789 | Envelope: 0.0457 | Mouth: half"
	if got != want {
		t.Errorf("audioDebugLabel2() = %q, want %q", got, want)
	}
}

// TestAudioDebugLabels_GateClosed は gate closed 時の表示確認 (1 行目)。
func TestAudioDebugLabels_GateClosed(t *testing.T) {
	s := NewState()
	s.AudioRMS = 0.0038
	s.AudioNoiseFloor = 0.0037
	s.AudioGateOpen = false
	got1 := audioDebugLabel1(s)
	if want := "Audio RMS: 0.0038 | Floor: 0.0037 | Gate: closed"; got1 != want {
		t.Errorf("audioDebugLabel1() = %q, want %q", got1, want)
	}
	// gate closed → Gated=0 になるはず (ゲーム側ロジックで決まる)
	s.AudioGatedRMS = 0
	s.AudioEnvelope = 0
	s.AudioMouthState = 0
	got2 := audioDebugLabel2(s)
	if want := "Gated: 0.0000 | Envelope: 0.0000 | Mouth: closed"; got2 != want {
		t.Errorf("audioDebugLabel2() = %q, want %q", got2, want)
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

// TestGateStateLabel は gate 状態ラベルの境界確認 (Phase 1.14.14)。
func TestGateStateLabel(t *testing.T) {
	tests := []struct {
		gateOpen bool
		want     string
	}{
		{true, "open"},
		{false, "closed"},
	}
	for _, tt := range tests {
		if got := gateStateLabel(tt.gateOpen); got != tt.want {
			t.Errorf("gateStateLabel(%v) = %q, want %q", tt.gateOpen, got, tt.want)
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
	panel := NewPanel(face, state, true, "")
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
	panel := NewPanel(face, state, true, "")
	panel.SetUIHidden(true)
	// nil image を渡しても uiHidden なら即 return するはず
	panel.Draw(nil)
}

// === Phase 1.14.16: 明示的 Save / Reset ボタン + Dirty flag ===

// TestState_DirtyInitiallyFalse は起動直後の Dirty が false であることを確認。
func TestState_DirtyInitiallyFalse(t *testing.T) {
	s := NewState()
	if s.Dirty {
		t.Error("NewState() should set Dirty=false")
	}
}

// TestNewPanel_SaveButtonInitiallyDisabled は Dirty=false 起動時に
// Save ボタンが disable で生成されることを確認 (Phase 1.14.16)。
//
// Round 3 で Reset ボタンは YAGNI 削除。Save ボタンのみ残す。
func TestNewPanel_SaveButtonInitiallyDisabled(t *testing.T) {
	face := LoadFontFace(16)
	state := NewState()
	panel := NewPanel(face, state, true, "")
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("NewPanel panicked: %v", r)
		}
	}()
	if panel.saveBtn == nil {
		t.Fatal("Save button should be created in NewPanel")
	}
	if !panel.saveBtn.GetWidget().Disabled {
		t.Error("Save button should be initially disabled (Dirty=false)")
	}
}

// TestNewPanel_ZeroDirtyState は起動時に statusLabel が空文字 (Dirty=false) で生成されることを確認。
func TestNewPanel_ZeroDirtyState(t *testing.T) {
	face := LoadFontFace(16)
	state := NewState()
	panel := NewPanel(face, state, true, "")
	if panel.statusLabel == nil {
		t.Fatal("statusLabel should be created in NewPanel")
	}
	if panel.statusMessage != "" {
		t.Errorf("statusMessage should be empty initially, got %q", panel.statusMessage)
	}
}

// TestSetStatus_UpdatesLabel は SetStatus が statusLabel.Label を更新することを確認。
func TestSetStatus_UpdatesLabel(t *testing.T) {
	face := LoadFontFace(16)
	state := NewState()
	panel := NewPanel(face, state, true, "")
	panel.SetStatus("saved")
	if panel.statusLabel.Label != "saved" {
		t.Errorf("statusLabel.Label: got %q want %q", panel.statusLabel.Label, "saved")
	}
	if panel.statusMessage != "saved" {
		t.Errorf("statusMessage: got %q want %q", panel.statusMessage, "saved")
	}
}

// TestNewPanel_AudioCheckboxTriState は audioEnabled=false で audioCheck が
// tri-state (WidgetGreyed) で生成されることを確認 (Phase 1.14.16 Critical #1 fix)。
//
// Round 3: Reset / RefreshWidgetsFromState 系は YAGNI 削除したので、
// このテストは audioCheck の tri-state 構築だけ検証する。
func TestNewPanel_AudioCheckboxTriState(t *testing.T) {
	face := LoadFontFace(16)
	state := NewState()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("NewPanel panicked: %v", r)
		}
	}()
	// audioEnabled=false → audioCheck は WidgetGreyed で生成
	NewPanel(face, state, false, "")
}

// === Phase 2.10.6: UpdateCameraStatus restart button tests ===

// TestUpdateCameraStatus_RestartableInLostSignal は Lost Signal 状態で
// Restart ボタンが有効になることを確認。
//
// Phase 2.10.6: mp_server.py は動いているが lastErr がある状態 (手覆い等) で
// ユーザーが手動再起動できるようにする。
func TestUpdateCameraStatus_RestartableInLostSignal(t *testing.T) {
	face := LoadFontFace(16)
	state := NewState()
	panel := NewPanel(face, state, true, "")

	// mpRunning=true, lastErr=non-nil → Lost Signal
	errMsg := "detection timeout"
	panel.UpdateCameraStatus(0, true, &errMsg, true)

	if panel.state.CameraMode != "Lost Signal" {
		t.Errorf("CameraMode = %q, want %q", panel.state.CameraMode, "Lost Signal")
	}
	if !panel.state.CameraRestartable {
		t.Error("CameraRestartable should be true in Lost Signal state")
	}
	if panel.cameraRestartBtn == nil {
		t.Fatal("cameraRestartBtn should exist")
	}
	if panel.cameraRestartBtn.GetWidget().Disabled {
		t.Error("Restart button should be enabled in Lost Signal state")
	}
}

// TestUpdateCameraStatus_RestartableInMouse は Mouse mode (エラーなし) で
// Restart ボタンが有効であることを確認 (全状態で常時クリック可能にするユーザー要求)。
func TestUpdateCameraStatus_RestartableInMouse(t *testing.T) {
	face := LoadFontFace(16)
	state := NewState()
	panel := NewPanel(face, state, true, "")

	// mpRunning=true, lastErr=nil, mode=0 (Mouse)
	panel.UpdateCameraStatus(0, true, nil, true)

	if panel.state.CameraMode != "Mouse" {
		t.Errorf("CameraMode = %q, want %q", panel.state.CameraMode, "Mouse")
	}
	if !panel.state.CameraRestartable {
		t.Error("CameraRestartable should be true in Mouse mode (always restartable)")
	}
	if panel.cameraRestartBtn == nil {
		t.Fatal("cameraRestartBtn should exist")
	}
	if panel.cameraRestartBtn.GetWidget().Disabled {
		t.Error("Restart button should be enabled in Mouse state")
	}
}

// TestUpdateCameraStatus_RestartableInActive は Active mode (カメラ使用中) で
// Restart ボタンが有効であることを確認 (全状態で常時クリック可能にするユーザー要求)。
func TestUpdateCameraStatus_RestartableInActive(t *testing.T) {
	face := LoadFontFace(16)
	state := NewState()
	panel := NewPanel(face, state, true, "")

	// mpRunning=true, lastErr=nil, mode=1 (Camera/Active)
	panel.UpdateCameraStatus(panelCameraModeCamera, true, nil, true)

	if panel.state.CameraMode != "Active" {
		t.Errorf("CameraMode = %q, want %q", panel.state.CameraMode, "Active")
	}
	if !panel.state.CameraRestartable {
		t.Error("CameraRestartable should be true in Active mode (always restartable)")
	}
	if panel.cameraRestartBtn == nil {
		t.Fatal("cameraRestartBtn should exist")
	}
	if panel.cameraRestartBtn.GetWidget().Disabled {
		t.Error("Restart button should be enabled in Active state")
	}
}

// TestUpdateCameraStatus_RestartableInDown は Down 状態で Restart ボタンが
// 有効であることを確認 (既存動作の回帰防止)。
func TestUpdateCameraStatus_RestartableInDown(t *testing.T) {
	face := LoadFontFace(16)
	state := NewState()
	panel := NewPanel(face, state, true, "")

	// mpRunning=false → Down
	panel.UpdateCameraStatus(0, false, nil, true)

	if panel.state.CameraMode != "Down" {
		t.Errorf("CameraMode = %q, want %q", panel.state.CameraMode, "Down")
	}
	if !panel.state.CameraRestartable {
		t.Error("CameraRestartable should be true in Down state")
	}
}

// === Phase 2.10.8: Camera Enabled OFF stops runtime tests ===

// TestUpdateCameraStatus_DisabledWhenCameraOff は CameraEnabled=false のとき
// "Disabled" 表示になり、Restart ボタンが無効になることを確認。
func TestUpdateCameraStatus_DisabledWhenCameraOff(t *testing.T) {
	face := LoadFontFace(16)
	state := NewState()
	panel := NewPanel(face, state, true, "")

	// cameraEnabled=false → どんな mode/mpRunning でも Disabled
	panel.UpdateCameraStatus(panelCameraModeCamera, true, nil, false)

	if panel.state.CameraMode != "Disabled" {
		t.Errorf("CameraMode = %q, want %q", panel.state.CameraMode, "Disabled")
	}
	if panel.state.CameraRestartable {
		t.Error("CameraRestartable should be false when camera is disabled")
	}
	if panel.cameraRestartBtn == nil {
		t.Fatal("cameraRestartBtn should exist")
	}
	if !panel.cameraRestartBtn.GetWidget().Disabled {
		t.Error("Restart button should be disabled when camera is disabled")
	}
	if panel.cameraStatusText == nil {
		t.Fatal("cameraStatusText should exist")
	}
	if panel.cameraStatusText.Label != "Camera: Disabled" {
		t.Errorf("cameraStatusText.Label = %q, want %q", panel.cameraStatusText.Label, "Camera: Disabled")
	}
}

// === Phase 2.10.8: Camera Enabled checkbox tests ===

// TestState_CameraEnabledDefault は CameraEnabled がデフォルト true であることを確認。
func TestState_CameraEnabledDefault(t *testing.T) {
	s := NewState()
	if !s.CameraEnabled {
		t.Error("CameraEnabled should default to true (Phase 2.10.8)")
	}
}

// TestNewPanel_CameraEnabledCheckbox は Camera Enabled チェックボックスが
// state.CameraEnabled に従って生成されることを確認。
func TestNewPanel_CameraEnabledCheckbox(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("NewPanel panicked: %v", r)
		}
	}()
	face := LoadFontFace(16)
	state := NewState()
	// CameraEnabled=true (default) → checkbox should be checked
	panel := NewPanel(face, state, true, "")
	if panel == nil {
		t.Fatal("expected non-nil panel")
	}
	// Phase 2.10.8 fix: cameraCheck が actually created されていることを確認
	if panel.cameraCheck == nil {
		t.Fatal("cameraCheck should be created in NewPanel")
	}
}

// TestNewPanel_CameraEnabledMinSize は Camera Enabled チェックボックスが
// 最小サイズ (16x16) で生成されることを確認 (Phase 2.10.8 fix)。
// MinSize がないと NewNineSliceColor の MinSize (0,0) により
// チェックボックスが 0x0 で描画され invisible になる。
func TestNewPanel_CameraEnabledMinSize(t *testing.T) {
	face := LoadFontFace(16)
	state := NewState()
	panel := NewPanel(face, state, true, "")
	if panel.cameraCheck == nil {
		t.Fatal("cameraCheck should be created")
	}
	w := panel.cameraCheck.GetWidget().MinWidth
	h := panel.cameraCheck.GetWidget().MinHeight
	if w < 16 || h < 16 {
		t.Errorf("cameraCheck MinSize = (%d, %d), want at least (16, 16)", w, h)
	}
}

// TestNewPanel_CameraEnabledDefaultChecked は CameraEnabled=true 時に
// チェックボックスが checked 状態で生成されることを確認。
func TestNewPanel_CameraEnabledDefaultChecked(t *testing.T) {
	face := LoadFontFace(16)
	state := NewState()
	if !state.CameraEnabled {
		t.Fatal("CameraEnabled should be true by default")
	}
	// NewPanel should not panic
	panel := NewPanel(face, state, true, "")
	if panel == nil {
		t.Fatal("expected non-nil panel")
	}
}

// === Phase 4.3: Morph Renderer UI tests ===

// TestState_MorphDefaults は Morph フィールドのデフォルト値を確認。
func TestState_MorphDefaults(t *testing.T) {
	s := NewState()
	if !s.MorphEnabled {
		t.Error("MorphEnabled should default to true")
	}
	if s.MorphStrength != 4.0 {
		t.Errorf("MorphStrength should default to 4.0 (Phase 4.3 hotfix), got %v", s.MorphStrength)
	}
	if s.TransitionDuration != 120.0 {
		t.Errorf("TransitionDuration should default to 120.0, got %v", s.TransitionDuration)
	}
}

// TestNewPanel_MorphUI は Morph UI が構築されることを確認。
func TestNewPanel_MorphUI(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("NewPanel panicked with morph UI: %v", r)
		}
	}()
	face := LoadFontFace(16)
	state := NewState()
	panel := NewPanel(face, state, true, "")
	if panel == nil {
		t.Fatal("expected non-nil panel")
	}
	if panel.morphStrengthLabel == nil {
		t.Error("morphStrengthLabel should be created in NewPanel")
	}
}

// TestNewPanel_MorphEnabledFalse は MorphEnabled=false で
// パネルが正しく構築されることを確認。
func TestNewPanel_MorphEnabledFalse(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("NewPanel panicked: %v", r)
		}
	}()
	face := LoadFontFace(16)
	state := NewState()
	state.MorphEnabled = false
	panel := NewPanel(face, state, true, "")
	if panel == nil {
		t.Fatal("expected non-nil panel even with MorphEnabled=false")
	}
}

// TestResetToDefaults_ResetsMorphFields は ResetToDefaults が Morph フィールドもリセットすることを確認。
func TestResetToDefaults_ResetsMorphFields(t *testing.T) {
	s := NewState()
	s.MorphEnabled = false
	s.MorphStrength = 12.0
	s.TransitionDuration = 150.0
	s.ResetToDefaults()
	if !s.MorphEnabled {
		t.Error("ResetToDefaults should set MorphEnabled=true")
	}
	if s.MorphStrength != 4.0 {
		t.Errorf("ResetToDefaults should set MorphStrength=4.0 (Phase 4.3 hotfix), got %v", s.MorphStrength)
	}
	if s.TransitionDuration != 120.0 {
		t.Errorf("ResetToDefaults should set TransitionDuration=120.0, got %v", s.TransitionDuration)
	}
}
