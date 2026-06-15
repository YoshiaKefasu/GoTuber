// Package character はキャラクター設定と画像読み込みを管理する。
package character

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config は 1 キャラクター分の設定を表す。
type Config struct {
	Name     string `yaml:"name"`
	BasePath string `yaml:"base_path"`
	Ext      string `yaml:"ext"`
	Rows     int    `yaml:"rows"`
	Cols     int    `yaml:"cols"`
	Sheets   Sheets `yaml:"sheets"`
}

// Sheets は 6 状態（目 × 口）のシート名マッピング。
type Sheets struct {
	EyesOpen   EyeMouthStates `yaml:"eyes_open"`
	EyesClosed EyeMouthStates `yaml:"eyes_closed"`
}

// EyeMouthStates は「目状態 × 口状態」の 3 値に対するシート名。
type EyeMouthStates struct {
	Closed string `yaml:"closed"`
	Half   string `yaml:"half"`
	Open   string `yaml:"open"`
}

// LoadConfig は YAML ファイルから設定を読み込む。
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	// デフォルト値
	if c.Rows == 0 {
		c.Rows = 5
	}
	if c.Cols == 0 {
		c.Cols = 5
	}
	if c.Ext == "" {
		c.Ext = "png"
	}
	return &c, nil
}

// SheetFor は目・口状態から対応するシート名を返す。
//   - eyesClosed: true (目閉じ) / false (目開け)
//   - mouth: 0=Closed, 1=Half, 2=Open
func (c *Config) SheetFor(eyesClosed bool, mouth int) string {
	eyeMap := c.Sheets.EyesOpen
	if eyesClosed {
		eyeMap = c.Sheets.EyesClosed
	}
	switch mouth {
	case 0:
		return eyeMap.Closed
	case 1:
		return eyeMap.Half
	case 2:
		return eyeMap.Open
	default:
		return eyeMap.Closed
	}
}

// PathFor はシート・行・列から画像ファイルパスを生成する。
func (c *Config) PathFor(sheet string, row, col int) string {
	return fmt.Sprintf("%s/%s/r%dc%d.%s", c.BasePath, sheet, row, col, c.Ext)
}
