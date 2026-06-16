package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
