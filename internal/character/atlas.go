package character

import (
	"fmt"
	"image"
	_ "image/png" // PNG 登録（init）
	"os"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
)

// Atlas は 5×5 × 6 状態 = 150 枚のスプライトシートを保持する。
// スレッドセーフ。Load() で全シートを並列デコード、Get() で遅延ロード。
type Atlas struct {
	cfg    *Config
	images [6][5][5]*ebiten.Image // 6 sheets × 5×5 grid
	loaded [6]bool                 // 各シートのロード完了フラグ

	mu    sync.RWMutex
	ready bool // 1 枚以上の画像がロードされたら true
}

// NewAtlas は設定から新しい Atlas を作成する（まだロードしない）。
func NewAtlas(cfg *Config) *Atlas {
	return &Atlas{cfg: cfg}
}

// Config は元設定を返す。
func (a *Atlas) Config() *Config { return a.cfg }

// Ready は 1 枚以上の画像がロードされたら true を返す。
func (a *Atlas) Ready() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.ready
}

// LoadAll は全 150 画像を並列デコードする（goroutine fan-out、semaphore で 4 並行）。
// Phase 1.3 では全画像を起動時にプリロード。Phase 1.4 で 1 シート + 隣接のみに変更予定。
func (a *Atlas) LoadAll() error {
	sheetNames := []string{"A", "B", "C", "D", "E", "F"}
	const concurrency = 4
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	errCh := make(chan error, 150)

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

	// 最初のエラーを返す（全部失敗しても 1 つだけ報告）
	var firstErr error
	for err := range errCh {
		if firstErr == nil {
			firstErr = err
		}
	}

	a.mu.Lock()
	a.ready = firstErr == nil
	a.mu.Unlock()

	return firstErr
}

// loadOne は 1 枚の画像を読み込んで Atlas に格納する。
func (a *Atlas) loadOne(sheetIdx, row, col int, sheet string) error {
	path := a.cfg.PathFor(sheet, row, col)
	img, err := loadImageFile(path)
	if err != nil {
		return fmt.Errorf("load %s: %w", path, err)
	}
	a.mu.Lock()
	a.images[sheetIdx][row][col] = img
	a.loaded[sheetIdx] = true
	a.ready = true
	a.mu.Unlock()
	return nil
}

// Get は指定位置の画像を返す。ロードされていなければ nil を返す（遅延ロードは未実装）。
func (a *Atlas) Get(sheetIdx, row, col int) *ebiten.Image {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.images[sheetIdx][row][col]
}

// SheetLoaded は指定シートのロード完了を返す。
func (a *Atlas) SheetLoaded(sheetIdx int) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.loaded[sheetIdx]
}

// loadImageFile はファイルを開いて image.Decode → ebiten.Image 変換する。
func loadImageFile(path string) (*ebiten.Image, error) {
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
