// Package character はキャラクター設定と画像読み込みを管理する。
package character

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

// SheetFor は目・口状態から対応するシート名（インデックス: 0-5）を返す。
//   - sheetIdx: 0=EyesOpen.Closed, 1=EyesOpen.Half, 2=EyesOpen.Open,
//     3=EyesClosed.Closed, 4=EyesClosed.Half, 5=EyesClosed.Open
//   - eyesClosed: true (目閉じ) / false (目開け)
//   - mouth: 0=Closed, 1=Half, 2=Open
func (c *Config) SheetFor(eyesClosed bool, mouth int) (sheetName string, sheetIdx int) {
	eyeMap := c.Sheets.EyesOpen
	eyesClosedSuffix := 0
	if eyesClosed {
		eyeMap = c.Sheets.EyesClosed
		eyesClosedSuffix = 3
	}
	switch mouth {
	case 0:
		return eyeMap.Closed, eyesClosedSuffix + 0
	case 1:
		return eyeMap.Half, eyesClosedSuffix + 1
	case 2:
		return eyeMap.Open, eyesClosedSuffix + 2
	default:
		return eyeMap.Closed, eyesClosedSuffix + 0
	}
}

// SheetNames は LoadAll が使う 6 シートの名前を config から返す。
func (c *Config) SheetNames() []string {
	return []string{
		c.Sheets.EyesOpen.Closed,
		c.Sheets.EyesOpen.Half,
		c.Sheets.EyesOpen.Open,
		c.Sheets.EyesClosed.Closed,
		c.Sheets.EyesClosed.Half,
		c.Sheets.EyesClosed.Open,
	}
}

// Validate は設定の妥当性をフェイルファストで検証する。
//   - base_path: 非空、`..` を含まない、存在するディレクトリ
//   - ext: "png" または "webp"
//   - 6 シートディレクトリ: 全て存在
//   - 各シートの 25 枚画像: 全て存在
func (c *Config) Validate() error {
	if c.BasePath == "" {
		return fmt.Errorf("empty base_path")
	}
	if c.Ext != "png" && c.Ext != "webp" {
		return fmt.Errorf("invalid ext: %s (must be png or webp)", c.Ext)
	}
	cleaned := filepath.Clean(c.BasePath)
	if strings.Contains(cleaned, "..") {
		return fmt.Errorf("invalid base_path: %s (contains ..)", c.BasePath)
	}
	info, err := os.Stat(cleaned)
	if err != nil {
		return fmt.Errorf("base_path not accessible: %s (%w)", cleaned, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("base_path is not a directory: %s", cleaned)
	}
	sheetNames := c.SheetNames()
	for _, name := range sheetNames {
		if name == "" {
			return fmt.Errorf("empty sheet name in config (check eyes_open/eyes_closed mapping)")
		}
		dir := filepath.Join(cleaned, name)
		if _, err := os.Stat(dir); err != nil {
			return fmt.Errorf("sheet directory missing: %s (%w)", dir, err)
		}
	}
	for _, name := range sheetNames {
		for r := 0; r < c.Rows; r++ {
			for col := 0; col < c.Cols; col++ {
				path := filepath.Join(cleaned, name, fmt.Sprintf("r%dc%d.%s", r, col, c.Ext))
				if _, err := os.Stat(path); err != nil {
					return fmt.Errorf("image missing: %s (%w)", path, err)
				}
			}
		}
	}
	return nil
}

// PathFor はシート・行・列から画像ファイルパスを生成する。
//   - 範囲外: error
//   - base_path に `..` を含む: error (path traversal 防止)
//   - base_path が空: error
func (c *Config) PathFor(sheet string, row, col int) (string, error) {
	if c.BasePath == "" {
		return "", fmt.Errorf("empty base_path")
	}
	if row < 0 || row >= c.Rows {
		return "", fmt.Errorf("row out of range: %d (rows=%d)", row, c.Rows)
	}
	if col < 0 || col >= c.Cols {
		return "", fmt.Errorf("col out of range: %d (cols=%d)", col, c.Cols)
	}
	cleaned := filepath.Clean(c.BasePath)
	if strings.Contains(cleaned, "..") {
		return "", fmt.Errorf("invalid base_path: %s (contains ..)", c.BasePath)
	}
	return filepath.Join(cleaned, sheet, fmt.Sprintf("r%dc%d.%s", row, col, c.Ext)), nil
}
