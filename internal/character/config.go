// Package character はキャラクター設定と画像読み込みを管理する。
//
// `src/character-config.js` の Go 移植版 (Phase 1.12 で完全 port)。
// 設定スキーマ・キー名 (`basePath`/`eyesOpen`/`eyesClosed`/`close`) は元と完全互換。
package character

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Config は 1 キャラクター分の設定を表す。
// src/character-config.js と完全互換のキー名 (camelCase YAML tags)。
type Config struct {
	BasePath string `yaml:"basePath"`
	Ext      string `yaml:"ext"`
	Rows     int    `yaml:"rows"`
	Cols     int    `yaml:"cols"`
	Sheets   Sheets `yaml:"sheets"`
}

// Sheets は 6 状態（目 × 口）のシート名マッピング。
// 元 `eyesOpen: { close, half, open }` / `eyesClosed: { close, half, open }` に対応。
type Sheets struct {
	EyesOpen   EyeMouthStates `yaml:"eyesOpen"`
	EyesClosed EyeMouthStates `yaml:"eyesClosed"`
}

// EyeMouthStates は「目状態 × 口状態」の 3 値に対するシート名。
// 元 character-config.js: `close: 'A'` に対応（`closed` ではなく `close`）。
type EyeMouthStates struct {
	Close string `yaml:"close"`
	Half  string `yaml:"half"`
	Open  string `yaml:"open"`
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
		c.Ext = "webp"
	}
	return &c, nil
}

// Src は sheet/row/col から画像ファイルパスを生成する。
// src/character-config.js:22-24 の `src(sheet, r, c)` メソッドの Go 移植。
func (c *Config) Src(sheet string, r, col int) string {
	return fmt.Sprintf("%s/%s/r%dc%d.%s", c.BasePath, sheet, r, col, c.Ext)
}

// SheetFor は目・口状態から対応するシート名（インデックス: 0-5）を返す。
//   - sheetIdx: 0=EyesOpen.Close, 1=EyesOpen.Half, 2=EyesOpen.Open,
//     3=EyesClosed.Close, 4=EyesClosed.Half, 5=EyesClosed.Open
//   - eyesClosed: true (目閉じ) / false (目開け)
//   - mouth: 0=Close, 1=Half, 2=Open
func (c *Config) SheetFor(eyesClosed bool, mouth int) (sheetName string, sheetIdx int) {
	eyeMap := c.Sheets.EyesOpen
	eyesClosedSuffix := 0
	if eyesClosed {
		eyeMap = c.Sheets.EyesClosed
		eyesClosedSuffix = 3
	}
	switch mouth {
	case 0:
		return eyeMap.Close, eyesClosedSuffix + 0
	case 1:
		return eyeMap.Half, eyesClosedSuffix + 1
	case 2:
		return eyeMap.Open, eyesClosedSuffix + 2
	default:
		return eyeMap.Close, eyesClosedSuffix + 0
	}
}

// SheetNames は LoadAll が使う 6 シートの名前を config から返す。
func (c *Config) SheetNames() []string {
	return []string{
		c.Sheets.EyesOpen.Close,   // "A"
		c.Sheets.EyesOpen.Half,    // "B"
		c.Sheets.EyesOpen.Open,    // "C"
		c.Sheets.EyesClosed.Close, // "D"
		c.Sheets.EyesClosed.Half,  // "E"
		c.Sheets.EyesClosed.Open,  // "F"
	}
}

// Validate は設定の妥当性をフェイルファストで検証する。
//   - basePath: 非空、`..` を **コンポーネント** として含まない、存在するディレクトリ
//   - ext: "png" または "webp"
//   - 6 シートディレクトリ: 全て存在
//   - 各シートの 25 枚画像: 全て存在（goroutine + semaphore で並列化）
func (c *Config) Validate() error {
	if c.BasePath == "" {
		return fmt.Errorf("empty basePath")
	}
	if c.Ext != "png" && c.Ext != "webp" {
		return fmt.Errorf("invalid ext: %s (must be png or webp)", c.Ext)
	}
	cleaned := filepath.Clean(c.BasePath)
	// パス **コンポーネント** 単位の traversal チェック
	// （"..backup" のような正規ディレクトリ名を誤検出しないため）
	for _, part := range strings.Split(filepath.ToSlash(cleaned), "/") {
		if part == ".." {
			return fmt.Errorf("invalid basePath: %s (path traversal)", c.BasePath)
		}
	}
	info, err := os.Stat(cleaned)
	if err != nil {
		return fmt.Errorf("basePath not accessible: %s (%w)", cleaned, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("basePath is not a directory: %s", cleaned)
	}
	sheetNames := c.SheetNames()
	for _, name := range sheetNames {
		if name == "" {
			return fmt.Errorf("empty sheet name in config (check eyesOpen/eyesClosed mapping)")
		}
		dir := filepath.Join(cleaned, name)
		if _, err := os.Stat(dir); err != nil {
			return fmt.Errorf("sheet directory missing: %s (%w)", dir, err)
		}
	}
	// 150 image check を並列化（goroutine + semaphore 8、SSD でも 5-15ms → 1-3ms へ短縮）
	const concurrency = 8
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	errCh := make(chan error, len(sheetNames)*c.Rows*c.Cols)
	for _, name := range sheetNames {
		for r := 0; r < c.Rows; r++ {
			for col := 0; col < c.Cols; col++ {
				wg.Add(1)
				sem <- struct{}{}
				go func(sheet string, row, col int) {
					defer wg.Done()
					defer func() { <-sem }()
					path := filepath.Join(cleaned, sheet, fmt.Sprintf("r%dc%d.%s", row, col, c.Ext))
					if _, err := os.Stat(path); err != nil {
						errCh <- fmt.Errorf("image missing: %s (%w)", path, err)
					}
				}(name, r, col)
			}
		}
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			return err
		}
	}
	return nil
}

// PathFor はシート・行・列から画像ファイルパスを生成する。
//   - 範囲外: error
//   - basePath に `..` を含む: error (path traversal 防止)
//   - basePath が空: error
func (c *Config) PathFor(sheet string, row, col int) (string, error) {
	if c.BasePath == "" {
		return "", fmt.Errorf("empty basePath")
	}
	if row < 0 || row >= c.Rows {
		return "", fmt.Errorf("row out of range: %d (rows=%d)", row, c.Rows)
	}
	if col < 0 || col >= c.Cols {
		return "", fmt.Errorf("col out of range: %d (cols=%d)", col, c.Cols)
	}
	cleaned := filepath.Clean(c.BasePath)
	if strings.Contains(cleaned, "..") {
		return "", fmt.Errorf("invalid basePath: %s (contains ..)", c.BasePath)
	}
	return filepath.Join(cleaned, sheet, fmt.Sprintf("r%dc%d.%s", row, col, c.Ext)), nil
}
