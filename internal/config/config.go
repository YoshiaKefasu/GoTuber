// Package config は GoTuber のユーザー設定 (TOML) を永続化する。
//
// 保存先: os.UserConfigDir() + "GoTuber/config.toml"
//   - Windows: %APPDATA%\GoTuber\config.toml
//   - Linux:   ~/.config/GoTuber/config.toml
//   - macOS:   ~/Library/Application Support/GoTuber/config.toml
//
// Phase 1.13a: マイクデバイス選択を永続化。TOML ライブラリは
// github.com/pelletier/go-toml/v2 (active maintenance, modern API)。
//
// Phase 1.14.16: Tweaks 4 フィールド (mouse_responsiveness / blink_enabled /
// mouth_enabled / mic_sensitivity) を `[tweaks]` セクションへ追加。明示的
// Save ボタン押下時のみ書き込む。ApplyTo はゼロ値を「TOML 未設定」とみなし skip。
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/YoshiaKefasu/GoTuber/internal/tweaks"
	"github.com/pelletier/go-toml/v2"
)

// Config は GoTuber のユーザー設定全体。
// トップレベルに [audio] など機能ごとのセクションをぶら下げる形。
type Config struct {
	// Audio は [audio] セクション。
	Audio AudioConfig `toml:"audio"`

	// Tweaks は [tweaks] セクション。Phase 1.14.16 で追加。
	// Save ボタン押下時のみ書き込まれ、起動時に ApplyTo 経由で State へ反映される。
	Tweaks TweaksConfig `toml:"tweaks"`
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

// TweaksConfig は [tweaks] セクションのフィールド (Phase 1.14.16)。
//
// TOML 上のフィールドはユーザーが編集可能だが、Go 側では State への
// 適用時 (ApplyTo) と TOML 書き出し時 (CaptureFrom) のみアクセスする。
// GUI (F1 Tweaks パネル) からの設定変更は state を経由し、Save ボタン押下
// で初めてここへコピーされる。
//
// Phase 1.14.16 (Critical #2 fix): BlinkEnabled / MouthEnabled は *bool。
// TOML に「キーが存在しない」 (nil) と「キーが存在するが false」 (非nil, false)
// を区別するため。bool のゼロ値 (= false) では区別不能で、初回起動時に
// ユーザーが明示的に OFF を選択していないのに口パク・まばたきが無効化される
// バグが発生する。
type TweaksConfig struct {
	// MouseResponsiveness: 0.05..1.0。int 値域 5..100 を内部で /100.0 換算。
	// ゼロ値は「TOML に書かれていない (= 未設定)」とみなして ApplyTo で skip する。
	MouseResponsiveness float64 `toml:"mouse_responsiveness"`

	// BlinkEnabled: 自動まばたき有効化。*bool で TOML キー欠落 (nil) と false を区別。
	// nil なら ApplyTo skip (State のデフォルト true を保持)。
	// 非 nil なら *b の値を State に上書き (明示的 OFF を尊重)。
	BlinkEnabled *bool `toml:"blink_enabled"`

	// MouthEnabled: 口パク有効化。*bool で TOML キー欠落 (nil) と false を区別。
	// nil なら ApplyTo skip (State のデフォルト true を保持)。
	// 非 nil なら *b の値を State に上書き (明示的 OFF を尊重)。
	MouthEnabled *bool `toml:"mouth_enabled"`

	// MicSensitivity: 1.0..20.0 (UI int 10..200 を内部で /10.0 換算)。
	// ゼロ値は「未設定」とみなし ApplyTo で skip。
	MicSensitivity float64 `toml:"mic_sensitivity"`

	// CameraEnabled: カメラ追跡モード有効化 (Phase 2.10.8)。
	// *bool で TOML キー欠落 (nil) と false を区別。
	// nil なら ApplyTo skip (State のデフォルト true を保持)。
	// 非 nil なら *b の値を State に上書き (明示的 OFF を尊重)。
	CameraEnabled *bool `toml:"camera_enabled"`

	// Phase 4.3: Morph Renderer 設定
	// MorphEnabled: depth-weighted elastic morph 有効化。*bool で nil と false を区別。
	MorphEnabled *bool `toml:"morph_enabled"`
	// MorphStrength: 最大変位量 (ピクセル)。0.0..8.0。
	// 0.0 は有効なユーザー設定なので *float64 で nil (キー欠落) と区別する。
	MorphStrength *float64 `toml:"morph_strength"`
	// TransitionDuration: セル切り替え transition 期間 (ms)。50..250。ゼロ値は skip。
	TransitionDuration float64 `toml:"transition_duration"`
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

// ApplyTo は TOML から読んだ値を state に上書きする。
//
// Phase 1.14.16: ゼロ値 / nil は「TOML に書かれていない (= 未設定)」とみなして skip。
//   - MouseResponsiveness: 0.0 なら skip (state のデフォルト 0.3 を保持)
//   - BlinkEnabled: *bool が nil なら skip (state のデフォルト true を保持)、
//     非 nil なら *b の値を上書き (明示的 OFF を尊重)
//   - MouthEnabled: *bool が nil なら skip (state のデフォルト true を保持)、
//     非 nil なら *b の値を上書き (明示的 OFF を尊重)
//   - MicSensitivity: 0.0 なら skip (state のデフォルト 10.0 を保持)
//   - CameraEnabled: *bool が nil なら skip (state のデフォルト true を保持)、
//     非 nil なら *b の値を上書き (明示的 OFF を尊重) (Phase 2.10.8)
//
// 呼び出しは cmd/gotuber/main.go の config.Load() 直後、NewState() 直後。
func (t *TweaksConfig) ApplyTo(state *tweaks.State) {
	if t.MouseResponsiveness != 0 {
		state.MouseResponsiveness = t.MouseResponsiveness
	}
	if t.BlinkEnabled != nil {
		state.BlinkEnabled = *t.BlinkEnabled
	}
	if t.MouthEnabled != nil {
		state.AudioEnabled = *t.MouthEnabled
	}
	if t.MicSensitivity != 0 {
		state.AudioSensitivity = t.MicSensitivity
	}
	if t.CameraEnabled != nil {
		state.CameraEnabled = *t.CameraEnabled
	}
	// Phase 4.3: Morph Renderer 設定
	if t.MorphEnabled != nil {
		state.MorphEnabled = *t.MorphEnabled
	}
	if t.MorphStrength != nil {
		state.MorphStrength = *t.MorphStrength
	}
	if t.TransitionDuration != 0 {
		state.TransitionDuration = t.TransitionDuration
	}
}

// CaptureFrom は state のフィールドを TOML 書き込み対象としてコピーする。
// Save ボタン押下時に main.go から呼ばれる。
//
// Phase 1.14.16: BlinkEnabled / MouthEnabled は *bool として必ずコピー (nil にしない)。
// Save ボタン押下 = ユーザーが明示的に Save を選択した瞬間なので、「明示的 OFF」
// と「TOML 欠落」を区別する必要はない。State の bool をそのまま & でラップ。
//
// Phase 2.10.8: CameraEnabled を追加。
// Phase 4.3: MorphEnabled / MorphStrength / TransitionDuration を追加。
func (t *TweaksConfig) CaptureFrom(state *tweaks.State) {
	t.MouseResponsiveness = state.MouseResponsiveness
	blinkVal := state.BlinkEnabled
	t.BlinkEnabled = &blinkVal
	mouthVal := state.AudioEnabled
	t.MouthEnabled = &mouthVal
	t.MicSensitivity = state.AudioSensitivity
	cameraVal := state.CameraEnabled
	t.CameraEnabled = &cameraVal
	// Phase 4.3
	morphVal := state.MorphEnabled
	t.MorphEnabled = &morphVal
	morphStrengthVal := state.MorphStrength
	t.MorphStrength = &morphStrengthVal
	t.TransitionDuration = state.TransitionDuration
}
