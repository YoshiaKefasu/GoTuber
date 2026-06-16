// Package config は GoTuber のユーザー設定 (TOML) を永続化する。
//
// 保存先: os.UserConfigDir() + "GoTuber/config.toml"
//   - Windows: %APPDATA%\GoTuber\config.toml
//   - Linux:   ~/.config/GoTuber/config.toml
//   - macOS:   ~/Library/Application Support/GoTuber/config.toml
//
// Phase 1.13a: マイクデバイス選択を永続化。TOML ライブラリは
// github.com/pelletier/go-toml/v2 (active maintenance, modern API)。
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

// Config は GoTuber のユーザー設定全体。
// トップレベルに [audio] など機能ごとのセクションをぶら下げる形。
type Config struct {
	// Audio は [audio] セクション。
	Audio AudioConfig `toml:"audio"`
}

// AudioConfig は [audio] セクションのフィールド。
type AudioConfig struct {
	// DeviceID は malgo 内部一意 ID (display name ではない)。
	// ListDevices() で取得した audio.Device.ID を保存する。
	// 空文字 = OS デフォルト。
	// 表示名が重複するデバイス (例: 同じ "USB Microphone" が複数)
	// 環境でも ID で一意に識別・復元できる。
	DeviceID string `toml:"device_id"`
}

// Path は設定ファイルの絶対パスを返す。
// ディレクトリは存在しなくても良い (Save 時に MkdirAll する)。
func Path() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("config: UserConfigDir: %w", err)
	}
	return filepath.Join(base, "GoTuber", "config.toml"), nil
}

// Load は設定ファイルを読み込み *Config を返す。
//
// Graceful degradation:
//   - ファイル未存在 (初回起動) → ゼロ値の *Config, nil
//   - 読み込み失敗・TOML パース失敗 → ゼロ値の *Config, error
//
// 呼び出し側は error を受け取っても &Config{} にフォールバックして
// 起動を続行する想定。P-4: 設定不備で起動失敗させない。
func Load() (*Config, error) {
	path, err := Path()
	if err != nil {
		return &Config{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// 初回起動: デフォルト設定で続行
			return &Config{}, nil
		}
		return &Config{}, fmt.Errorf("config: read %s: %w", path, err)
	}
	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return &Config{}, fmt.Errorf("config: parse %s: %w", path, err)
	}
	return &cfg, nil
}

// Save は設定を TOML 形式でファイルに書き込む。
// 親ディレクトリが存在しない場合は作成する (0755)。
// ファイルは 0644 で作成される。
func (c *Config) Save() error {
	path, err := Path()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("config: mkdir %s: %w", dir, err)
	}
	data, err := toml.Marshal(c)
	if err != nil {
		return fmt.Errorf("config: marshal: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("config: write %s: %w", path, err)
	}
	return nil
}
