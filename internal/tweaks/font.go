// Package tweaks は Tweaks パネル UI を提供する。
//
// ebitenui v0.7.3 + Gen Interface JP Regular (SIL OFL 1.1) ベース。
// F1 キーで表示切替。マウス追従・まばたき・口パク・終了の動的制御。
package tweaks

import (
	"bytes"
	_ "embed"

	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

//go:embed assets/fonts/GenInterfaceJP-Regular.ttf
var fontBytes []byte

// LoadFontFace は埋め込みフォントから *text.GoTextFace を生成する。
// size: ピクセル単位の文字サイズ
// ebitenui は *text.Face を要求するため、ポインタを返す。
func LoadFontFace(size float64) *text.GoTextFace {
	source, err := text.NewGoTextFaceSource(bytes.NewReader(fontBytes))
	if err != nil {
		// 埋め込みフォントのロード失敗は起動時に致命的
		panic("tweaks: failed to load font: " + err.Error())
	}
	return &text.GoTextFace{
		Source: source,
		Size:   size,
	}
}
