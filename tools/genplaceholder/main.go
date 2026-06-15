// genplaceholder は Phase 1.3 テスト用の placeholder キャラクター画像を生成する。
// 使い方: go run ./tools/genplaceholder
//
// 6 sheets (A-F) × 5×5 cells = 150 PNG (200x200 単色)
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
)

const (
	baseDir  = "assets/characters/_default"
	cellSize = 200
)

var sheets = map[string]color.RGBA{
	"A": {100, 150, 255, 255}, // 目開け・口とじ (blue)
	"B": {100, 255, 150, 255}, // 目開け・口中間 (green)
	"C": {255, 100, 100, 255}, // 目開け・口開け (red)
	"D": {150, 100, 255, 255}, // 目閉じ・口とじ (purple)
	"E": {255, 200, 100, 255}, // 目閉じ・口中間 (orange)
	"F": {100, 255, 255, 255}, // 目閉じ・口開け (cyan)
}

func main() {
	for sheet, c := range sheets {
		sheetDir := fmt.Sprintf("%s/%s", baseDir, sheet)
		if err := os.MkdirAll(sheetDir, 0o755); err != nil {
			panic(err)
		}
		for r := 0; r < 5; r++ {
			for col := 0; col < 5; col++ {
				img := image.NewRGBA(image.Rect(0, 0, cellSize, cellSize))
				for y := 0; y < cellSize; y++ {
					for x := 0; x < cellSize; x++ {
						img.Set(x, y, c)
					}
				}
				filename := fmt.Sprintf("%s/r%dc%d.png", sheetDir, r, col)
				f, err := os.Create(filename)
				if err != nil {
					panic(err)
				}
				if err := png.Encode(f, img); err != nil {
					panic(err)
				}
				f.Close()
			}
		}
		fmt.Printf("Generated %s: 25 images\n", sheet)
	}
	fmt.Printf("Done. Total: %d images in %s\n", len(sheets)*25, baseDir)
}
