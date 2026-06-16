package character

import (
	"fmt"
	"image"
	_ "image/png" // PNG 登録（init）
	"os"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	_ "golang.org/x/image/webp" // WebP 登録（init、Phase 1.4 で追加）
)

// Atlas は 5×5 × 6 状態 = 150 枚のスプライトシートを保持する。
// スレッドセーフ。LoadAll() で全シートを並列デコード、Get() で画像取得。
type Atlas struct {
	cfg    *Config
	images [6][5][5]*ebiten.Image // 6 sheets × 5×5 grid (Phase 1.4 時点は固定長。動的化は別 Issue)
	loaded [6]bool                 // 各シートのロード完了フラグ (@todo lazy decode: Phase 1.5 以降で再評価)

	mu      sync.RWMutex
	ready   bool   // 1 枚以上の画像がロードされたら true
	lastErr error  // 最後に発生したエラー（nil なら成功）
}

// NewAtlas は設定から新しい Atlas を作成する（まだロードしない）。
func NewAtlas(cfg *Config) *Atlas {
	return &Atlas{cfg: cfg}
}

// Ready は 1 枚以上の画像がロードされたら true を返す。
func (a *Atlas) Ready() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.ready
}

// LastErr は最後に発生したエラーを返す。nil なら全画像ロード成功。
func (a *Atlas) LastErr() error {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.lastErr
}

// LoadAll は全 150 画像を並列デコードする（goroutine fan-out、semaphore で 4 並行）。
// Phase 1.3 では全画像を起動時にプリロード。Phase 1.4 で 1 シート + 隣接のみに変更予定。
func (a *Atlas) LoadAll() error {
	sheetNames := a.cfg.SheetNames()
	const concurrency = 4
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	errCh := make(chan error, len(sheetNames)*a.cfg.Rows*a.cfg.Cols)

	for sheetIdx, name := range sheetNames {
		for r := 0; r < a.cfg.Rows; r++ {
			for col := 0; col < a.cfg.Cols; col++ {
				wg.Add(1)
				sem <- struct{}{}
				go func(idx int, row, c int, sheet string) {
					defer wg.Done()
					defer func() { <-sem }()
					if err := a.loadOne(idx, row, c, sheet); err != nil {
						errCh <- err
					}
				}(sheetIdx, r, col, name)
			}
		}
	}
	wg.Wait()
	close(errCh)

	// 最初のエラーを採用（1 つ報告すれば十分）
	var firstErr error
	for err := range errCh {
		if firstErr == nil {
			firstErr = err
		}
	}

	// ready は loadOne 内で画像成功時に true になる（上書きしない → フリッカ防止）
	// lastErr だけ更新
	a.mu.Lock()
	a.lastErr = firstErr
	a.mu.Unlock()

	return firstErr
}

// loadOne は 1 枚の画像を読み込んで Atlas に格納する。
func (a *Atlas) loadOne(sheetIdx, row, col int, sheet string) error {
	path, err := a.cfg.PathFor(sheet, row, col)
	if err != nil {
		return fmt.Errorf("path for %s: %w", sheet, err)
	}
	img, err := loadImageFile(path)
	if err != nil {
		return fmt.Errorf("load %s: %w", path, err)
	}
	a.mu.Lock()
	a.images[sheetIdx][row][col] = img
	a.loaded[sheetIdx] = true
	a.ready = true // 1 枚でもロードできれば ready にする
	a.mu.Unlock()
	return nil
}

// Get は指定位置の画像を返す。
//   - 第 2 戻り値: 範囲内なら true、範囲外なら false
//   - 範囲内だが未ロード: (nil, true) （Phase 1.3 では全ロード済のため発生しない）
func (a *Atlas) Get(sheetIdx, row, col int) (*ebiten.Image, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if sheetIdx < 0 || sheetIdx >= 6 || row < 0 || row >= a.cfg.Rows || col < 0 || col >= a.cfg.Cols {
		return nil, false
	}
	return a.images[sheetIdx][row][col], true
}

// SheetFor は (eyesClosed, mouth) に対応するシート index を返す。
// 内部の Config.SheetFor に委譲。Game の sheetForState() から呼ばれる。
//   - eyesClosed: false=目開け (A/B/C), true=目閉じ (D/E/F)
//   - mouth: 0=Close, 1=Half, 2=Open
func (a *Atlas) SheetFor(eyesClosed bool, mouth int) (sheetName string, sheetIdx int) {
	return a.cfg.SheetFor(eyesClosed, mouth)
}

// loadImageFile はファイルを開いて image.Decode → ebiten.Image 変換する。
// 16 MB を超えるファイルはエラー（DoS 対策）。
func loadImageFile(path string) (*ebiten.Image, error) {
	const maxImageSize = 16 << 20 // 16 MB
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > maxImageSize {
		return nil, fmt.Errorf("file too large: %d bytes (max %d)", info.Size(), maxImageSize)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}
	return ebiten.NewImageFromImage(img), nil
}
