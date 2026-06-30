package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/YoshiaKefasu/GoTuber/internal/tweaks"
	"github.com/pelletier/go-toml/v2"
)

// withTempConfigHome は os.UserConfigDir() が指すディレクトリを
// テスト用の一時ディレクトリに差し替える。t.Cleanup で自動復元。
//
// Windows: %APPDATA% を一時ディレクトリに
// Linux/macOS: $XDG_CONFIG_HOME を一時ディレクトリに (フォールバック)
func withTempConfigHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	// Windows は APPDATA、Linux は XDG_CONFIG_HOME が os.UserConfigDir() で読まれる。
	// t.Setenv は t.Cleanup で自動復元される。
	t.Setenv("APPDATA", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	return dir
}

// TestConfig_DefaultIsEmpty は Config のゼロ値が空の DeviceID を持つことを確認。
func TestConfig_DefaultIsEmpty(t *testing.T) {
	cfg := &Config{}
	if cfg.Audio.DeviceID != "" {
		t.Errorf("expected default Audio.DeviceID=\"\", got %q", cfg.Audio.DeviceID)
	}
}

// TestConfig_TOMLRoundTrip は Marshal → Unmarshal で完全に復元できることを確認。
func TestConfig_TOMLRoundTrip(t *testing.T) {
	original := &Config{
		Audio: AudioConfig{
			DeviceID: "{0.0.0.00000000}.{abc-def-1234-5678-abcd}",
		},
	}
	data, err := toml.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), "device_id") {
		t.Errorf("expected serialized TOML to contain 'device_id' key, got: %s", string(data))
	}
	if !strings.Contains(string(data), "abc-def-1234-5678-abcd") {
		t.Errorf("expected serialized TOML to contain device ID, got: %s", string(data))
	}

	var loaded Config
	if err := toml.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if loaded.Audio.DeviceID != original.Audio.DeviceID {
		t.Errorf("round-trip mismatch: expected %q, got %q",
			original.Audio.DeviceID, loaded.Audio.DeviceID)
	}
}

// TestConfig_LoadWhenNotExists は初回起動 (ファイル未存在) でデフォルト設定 + nil エラーを返すことを確認。
func TestConfig_LoadWhenNotExists(t *testing.T) {
	withTempConfigHome(t)
	cfg, err := Load()
	if err != nil {
		t.Errorf("expected nil error for first launch, got %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil Config")
	}
	if cfg.Audio.DeviceID != "" {
		t.Errorf("expected default empty DeviceID, got %q", cfg.Audio.DeviceID)
	}
}

// TestConfig_SaveAndLoad は Save → Load で設定が保存・復元できることを確認。
func TestConfig_SaveAndLoad(t *testing.T) {
	withTempConfigHome(t)

	original := &Config{
		Audio: AudioConfig{DeviceID: "test-device-id-12345"},
	}
	if err := original.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// ファイル存在確認
	path, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected config file to exist at %s: %v", path, err)
	}

	// ファイル内容の sanity check
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), "test-device-id-12345") {
		t.Errorf("config file should contain device ID, got: %s", string(data))
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Audio.DeviceID != original.Audio.DeviceID {
		t.Errorf("expected %q, got %q", original.Audio.DeviceID, loaded.Audio.DeviceID)
	}
}

// TestConfig_LoadInvalidTOML は壊れた TOML で graceful degradation (空 Config + error) を確認。
func TestConfig_LoadInvalidTOML(t *testing.T) {
	dir := withTempConfigHome(t)
	goTuberDir := filepath.Join(dir, "GoTuber")
	if err := os.MkdirAll(goTuberDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(goTuberDir, "config.toml")
	if err := os.WriteFile(path, []byte("not valid toml = = = ="), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	cfg, err := Load()
	if err == nil {
		t.Error("expected error for invalid TOML, got nil")
	}
	if cfg == nil {
		t.Fatal("expected non-nil Config even on error (graceful degradation)")
	}
	if cfg.Audio.DeviceID != "" {
		t.Errorf("expected default empty DeviceID on error, got %q", cfg.Audio.DeviceID)
	}
}

// TestConfig_SaveCreatesDirectory は Save が親ディレクトリを自動作成することを確認。
func TestConfig_SaveCreatesDirectory(t *testing.T) {
	withTempConfigHome(t)

	cfg := &Config{Audio: AudioConfig{DeviceID: "auto-mkdir-test"}}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	path, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	// ファイル存在 + 親ディレクトリ存在
	dir := filepath.Dir(path)
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("expected directory %s to exist: %v", dir, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected config file at %s: %v", path, err)
	}
}

// TestConfig_TweaksRoundTrip は Tweaks 5 フィールドが TOML 経由で完全に保存・復元できることを確認 (Phase 1.14.16 + Phase 2.10.8)。
//
// Phase 1.14.16 (Critical #2 fix): BlinkEnabled / MouthEnabled は *bool として保存される。
// Phase 2.10.8: CameraEnabled を追加。
func TestConfig_TweaksRoundTrip(t *testing.T) {
	withTempConfigHome(t)

	trueVal := true
	falseVal := false
	original := &Config{
		Audio: AudioConfig{DeviceID: "test-device"},
		Tweaks: TweaksConfig{
			MouseResponsiveness: 0.8,
			BlinkEnabled:        &trueVal,
			MouthEnabled:        &falseVal,
			MicSensitivity:      12.5,
			CameraEnabled:       &falseVal,
		},
	}
	if err := original.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Audio.DeviceID != "test-device" {
		t.Errorf("audio.device_id: got %q want %q", loaded.Audio.DeviceID, "test-device")
	}
	if loaded.Tweaks.MouseResponsiveness != 0.8 {
		t.Errorf("tweaks.mouse_responsiveness: got %v want 0.8", loaded.Tweaks.MouseResponsiveness)
	}
	if loaded.Tweaks.BlinkEnabled == nil || *loaded.Tweaks.BlinkEnabled != true {
		t.Errorf("tweaks.blink_enabled: got %v want true", loaded.Tweaks.BlinkEnabled)
	}
	if loaded.Tweaks.MouthEnabled == nil || *loaded.Tweaks.MouthEnabled != false {
		t.Errorf("tweaks.mouth_enabled: got %v want false", loaded.Tweaks.MouthEnabled)
	}
	if loaded.Tweaks.MicSensitivity != 12.5 {
		t.Errorf("tweaks.mic_sensitivity: got %v want 12.5", loaded.Tweaks.MicSensitivity)
	}
	if loaded.Tweaks.CameraEnabled == nil || *loaded.Tweaks.CameraEnabled != false {
		t.Errorf("tweaks.camera_enabled: got %v want false", loaded.Tweaks.CameraEnabled)
	}
}

// TestTweaksConfig_ApplyTo_ZeroValueSkip はゼロ値 / nil (TOML 未設定) が ApplyTo で skip されることを確認 (Phase 1.14.16)。
//
// Phase 1.14.16 (Critical #2 fix): BlinkEnabled / MouthEnabled が nil (TOML キー欠落)
// のときは State のデフォルト (true / true) を保持する。false の上書きは行わない。
// これが「初回起動時に口パク OFF にされてしまう」バグを防ぐ肝。
//
// Phase 2.10.8: CameraEnabled も nil skip でデフォルト true を保持。
func TestTweaksConfig_ApplyTo_ZeroValueSkip(t *testing.T) {
	// デフォルト state (0.3 / true / true / 10.0 / true)
	state := &tweaks.State{
		MouseResponsiveness: 0.3,
		BlinkEnabled:        true,
		AudioEnabled:        true,
		AudioSensitivity:    10.0,
		CameraEnabled:       true,
		MorphEnabled:        true,
		MorphStrength:       8.0,
		TransitionDuration:  100.0,
	}

	// 全フィールドゼロ値 / nil の TweaksConfig を ApplyTo
	tc := &TweaksConfig{}
	tc.ApplyTo(state)

	// ゼロ値 / nil skip: 全永続化フィールドがデフォルトのまま
	if state.MouseResponsiveness != 0.3 {
		t.Errorf("MouseResponsiveness should remain 0.3, got %v", state.MouseResponsiveness)
	}
	if !state.BlinkEnabled {
		t.Errorf("BlinkEnabled should remain true (TOML キー欠落 = skip), got false")
	}
	if !state.AudioEnabled {
		t.Errorf("AudioEnabled should remain true (TOML キー欠落 = skip), got false")
	}
	if state.AudioSensitivity != 10.0 {
		t.Errorf("AudioSensitivity should remain 10.0, got %v", state.AudioSensitivity)
	}
	if !state.CameraEnabled {
		t.Errorf("CameraEnabled should remain true (TOML キー欠落 = skip), got false")
	}
	if !state.MorphEnabled {
		t.Errorf("MorphEnabled should remain true (TOML キー欠落 = skip), got false")
	}
	if state.MorphStrength != 8.0 {
		t.Errorf("MorphStrength should remain 8.0, got %v", state.MorphStrength)
	}
	if state.TransitionDuration != 100.0 {
		t.Errorf("TransitionDuration should remain 100.0, got %v", state.TransitionDuration)
	}
}

// TestTweaksConfig_ApplyTo_ExplicitFalseRespected は *bool が false を指しているときに
// 明示的 OFF として state を false に上書きすることを確認 (Phase 1.14.16)。
//
// Phase 2.10.8: CameraEnabled の明示的 OFF も確認。
func TestTweaksConfig_ApplyTo_ExplicitFalseRespected(t *testing.T) {
	state := &tweaks.State{
		BlinkEnabled:  true,
		AudioEnabled:  true,
		CameraEnabled: true,
	}
	falseVal := false
	tc := &TweaksConfig{
		BlinkEnabled:  &falseVal,
		MouthEnabled:  &falseVal,
		CameraEnabled: &falseVal,
	}
	tc.ApplyTo(state)
	if state.BlinkEnabled {
		t.Error("BlinkEnabled should be overridden to false (明示的 OFF)")
	}
	if state.AudioEnabled {
		t.Error("AudioEnabled should be overridden to false (明示的 OFF)")
	}
	if state.CameraEnabled {
		t.Error("CameraEnabled should be overridden to false (明示的 OFF)")
	}
}

// TestTweaksConfig_CaptureFrom は state の 5 フィールドが TOML 書き込み対象としてコピーされることを確認 (Phase 1.14.16 + Phase 2.10.8)。
//
// Phase 1.14.16 (Critical #2 fix): BlinkEnabled / MouthEnabled は *bool にラップして
// 必ず TOML に書き込む (= Save 押下後は nil にならない)。
// Phase 2.10.8: CameraEnabled を追加。
func TestTweaksConfig_CaptureFrom(t *testing.T) {
	state := &tweaks.State{
		MouseResponsiveness: 0.5,
		BlinkEnabled:        false,
		AudioEnabled:        false,
		AudioSensitivity:    15.0,
		CameraEnabled:       false,
		MorphEnabled:        true,
		MorphStrength:       0,
		TransitionDuration:  150.0,
	}

	tc := &TweaksConfig{}
	tc.CaptureFrom(state)

	if tc.MouseResponsiveness != 0.5 {
		t.Errorf("MouseResponsiveness: got %v want 0.5", tc.MouseResponsiveness)
	}
	if tc.BlinkEnabled == nil || *tc.BlinkEnabled {
		t.Errorf("BlinkEnabled: got %v want false pointer", tc.BlinkEnabled)
	}
	if tc.MouthEnabled == nil || *tc.MouthEnabled {
		t.Errorf("MouthEnabled: got %v want false pointer", tc.MouthEnabled)
	}
	if tc.MicSensitivity != 15.0 {
		t.Errorf("MicSensitivity: got %v want 15.0", tc.MicSensitivity)
	}
	if tc.CameraEnabled == nil || *tc.CameraEnabled {
		t.Errorf("CameraEnabled: got %v want false pointer", tc.CameraEnabled)
	}
	if tc.MorphEnabled == nil || *tc.MorphEnabled != true {
		t.Errorf("MorphEnabled: got %v want true pointer", tc.MorphEnabled)
	}
	if tc.MorphStrength == nil || *tc.MorphStrength != 0 {
		t.Errorf("MorphStrength: got %v want 0 pointer", tc.MorphStrength)
	}
	if tc.TransitionDuration != 150.0 {
		t.Errorf("TransitionDuration: got %v want 150.0", tc.TransitionDuration)
	}
}

func TestTweaksConfig_MorphStrengthZeroPersists(t *testing.T) {
	state := &tweaks.State{MorphStrength: 0}
	tc := &TweaksConfig{}
	tc.CaptureFrom(state)
	if tc.MorphStrength == nil || *tc.MorphStrength != 0 {
		t.Fatalf("CaptureFrom MorphStrength: got %v want 0 pointer", tc.MorphStrength)
	}

	loadedState := &tweaks.State{MorphStrength: 8.0}
	tc.ApplyTo(loadedState)
	if loadedState.MorphStrength != 0 {
		t.Fatalf("ApplyTo MorphStrength: got %v want 0", loadedState.MorphStrength)
	}
}
