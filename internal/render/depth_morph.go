// Package render — Phase 4.2: depth-weighted elastic morph
//
// depth map を使ってメッシュ頂点を変位させ、キャラクター画像に
// ゆっくりとした向き変化の追従（elastic morph）を加える。
//
// depth map の意味:
//   - 白 (255) = 手前 → 頂点変位の影響を強く受ける
//   - 黒 (0)   = 奥   → 頂点変位の影響を弱くする
//
// smoothing: EMA (Exponential Moving Average) を採用。
// 理由: spring + damping は揺れ感が調整しやすいが、最小実装では
// パラメータが 1 つ少ない EMA が妥当。将来の拡張ポイントとして
// spring への切替は容易。
package render

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/png" // PNG decoder registration
	"math"
	"os"
	"sync"
)

// ─── Morph 設定 ──────────────────────────────────────────────────────────────

const (
	// morphStrength は最大変位量（ピクセル）。
	// 1200x1200 画像が ~648px に縮小されるため、控えめに 8px 程度。
	// 過大だと輪郭が破綻する。Tweaks UI で調整可能にするのは Phase 4.3。
	morphStrength = 8.0

	// morphSmoothing は EMA の smoothing factor (0.0〜1.0)。
	// 小さいほど追従が遅い（elastic になる）。
	// 0.15 は「60fps で ~10 フレーム程度の遅延」に相当。
	// 理由: spring + damping はパラメータ 2 つ (k, b) が必要だが、
	// 最小実装では EMA (1 パラメータ) で十分。将来の拡張ポイント。
	morphSmoothing = 0.15
)

// ─── Elastic 状態 ────────────────────────────────────────────────────────────

// MorphElastic は elastic morph の滑らか追従状態を保持する。
// Game が 1 つ保持し、毎フレーム UpdateMorphElastic で更新する。
type MorphElastic struct {
	ElX, ElY float64 // 現在の elastic 変位 (スクリーンピクセル)
}

// UpdateMorphElastic は elastic 状態を 1 フレーム分 EMA で更新する。
//
// 引数:
//   - e:        現在の elastic 状態（更新される）
//   - targetX:  目標変位 X (スクリーンピクセル、正=右)
//   - targetY:  目標変位 Y (スクリーンピクセル、正=下)
//
// 純粋関数なのでユニットテストで検証可能。
func UpdateMorphElastic(e *MorphElastic, targetX, targetY float64) {
	e.ElX += (targetX - e.ElX) * morphSmoothing
	e.ElY += (targetY - e.ElY) * morphSmoothing
}

// ─── Depth map 読み込み + キャッシュ ────────────────────────────────────────

var (
	depthCache   = make(map[string]*image.Gray)
	depthCacheMu sync.RWMutex
)

// LoadDepthMap は depth map PNG を grayscale として読み込み、キャッシュする。
// キャッシュにある場合はディスク I/O なしで返す。
//
// depth map が存在しない/読めない場合は (nil, false) を返す。
// 呼び出し側は false のとき flat mesh に fallback する。
func LoadDepthMap(path string) (*image.Gray, bool) {
	if path == "" {
		return nil, false
	}

	// キャッシュ確認
	depthCacheMu.RLock()
	cached, ok := depthCache[path]
	depthCacheMu.RUnlock()
	if ok {
		return cached, true
	}

	// ファイル読み込み
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, false
	}

	// grayscale に変換（RGBA / NRGBA / 既に Gray など、形式を統一）
	gray := toGray(img)

	// キャッシュ保存
	depthCacheMu.Lock()
	depthCache[path] = gray
	depthCacheMu.Unlock()

	return gray, true
}

// toGray は任意の image.Image を *image.Gray に変換する。
// alpha がある場合、alpha=0 のピクセルは depth=0 (黒) に確定
// (PHASE3.md 3.6.3b: 背景クリア仕様に準拠)。
func toGray(img image.Image) *image.Gray {
	bounds := img.Bounds()
	gray := image.NewGray(bounds)
	draw.Draw(gray, bounds, image.Black, image.Point{}, draw.Src)

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			if a == 0 {
				// 背景ピクセル: depth=0 (黒) に固定
				gray.SetGray(x, y, color.Gray{Y: 0})
				continue
			}
			// sRGB → grayscale 計算 (ITU-R BT.601)
			lum := uint8((0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)) / 256.0)
			gray.SetGray(x, y, color.Gray{Y: lum})
		}
	}
	return gray
}

// SampleDepth は depth map の指定 UV 座標 (0.0〜1.0) から深度値をサンプリングする。
// 戻り値: 0.0 (黒=奥) 〜 1.0 (白=手前)。
//
// 純粋関数なのでユニットテストで検証可能。
func SampleDepth(depthMap *image.Gray, u, v float64) float64 {
	if depthMap == nil {
		return 0
	}
	bounds := depthMap.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()
	if w == 0 || h == 0 {
		return 0
	}

	// UV → ピクセル座標 (clamped)
	px := int(u * float64(w))
	py := int(v * float64(h))
	if px < 0 {
		px = 0
	}
	if px >= w {
		px = w - 1
	}
	if py < 0 {
		py = 0
	}
	if py >= h {
		py = h - 1
	}

	c := depthMap.GrayAt(px+bounds.Min.X, py+bounds.Min.Y)
	return float64(c.Y) / 255.0
}

// ─── Morph メッシュ生成 ──────────────────────────────────────────────────────

// MorphParams は morph 計算に必要な全パラメータをまとめる。
type MorphParams struct {
	DepthMap *image.Gray  // depth map (nil なら flat mesh に fallback)
	ElX     float64      // 現在の elastic 変位 X (スクリーンピクセル)
	ElY     float64      // 現在の elastic 変位 Y (スクリーンピクセル)
	Alpha   float64      // アルファ値 (0.0〜1.0)
}

// GenerateMorphedMesh は depth map による頂点変位を適用したメッシュを生成する。
//
// 処理:
//  1. GenerateFlatMesh でフラットメッシュを生成
//  2. 各頂点の UV 座標で depth map をサンプリング
//  3. depth 値に応じて DstX/DstY を変位させる
//  4. 変位量 = depth * elastic * strength * weight(u,v)
//
// depthMap が nil の場合は flat mesh をそのまま返す (fallback)。
func GenerateMorphedMesh(imgW, imgH, screenW, screenH float64, params MorphParams) *MeshGrid {
	mesh := GenerateFlatMesh(imgW, imgH, screenW, screenH, params.Alpha)

	if params.DepthMap == nil {
		return mesh
	}

	// flat mesh の頂点座標をコピーして変位を適用
	for i := range mesh.Vertices {
		v := &mesh.Vertices[i]

		// UV 座標 (0.0〜1.0)
		u := float64(v.SrcX) / float64(imgW)
		uv := float64(v.SrcY) / float64(imgH)

		// depth map から深度値をサンプリング
		depth := SampleDepth(params.DepthMap, u, uv)

		// 画面全体が平行移動しないよう、追加重みを適用
		//   - 中央 (u=0.5, v=0.5): weight ≈ 0.0 (ほぼ動かない)
		//   - 周辺 (u/v=0 or 1):   weight ≈ 1.0 (最大変位)
		//   - 顔の中央付近は動かしすぎない
		weight := edgeWeight(u, uv)

		// 変位量 = depth * elastic * strength * weight
		offsetX := depth * params.ElX * (morphStrength / 30.0) * weight
		offsetY := depth * params.ElY * (morphStrength / 30.0) * weight

		v.DstX += float32(offsetX)
		v.DstY += float32(offsetY)
	}

	return mesh
}

// edgeWeight は UV 座標から周辺重みを返す。
// 中央 (0.5, 0.5) で 0.0、端 (0 or 1) で 1.0 に近づく。
// 画面全体の平行移動を防ぐための重み。
//
// 純粋関数なのでユニットテストで検証可能。
func edgeWeight(u, v float64) float64 {
	// 中心からの距離 (0.0=中心, 最大 ~0.707=角)
	du := u - 0.5
	dv := v - 0.5
	dist := math.Sqrt(du*du + dv*dv)

	// dist=0 → 0.0, dist>=0.5 → 1.0 に clamped
	w := dist / 0.5
	if w > 1.0 {
		w = 1.0
	}
	return w
}

// ─── ヘルパー ────────────────────────────────────────────────────────────────

// DepthMapPath は character ディレクトリから depth map パスを構築する。
// パス形式: `{charDir}/{sheet}/depth/r{row}c{col}.png`
//
// 純粋関数なのでユニットテストで検証可能。
func DepthMapPath(charDir, sheet string, row, col int) string {
	return fmt.Sprintf("%s/%s/depth/r%dc%d.png", charDir, sheet, row, col)
}

// InvalidateDepthCache は depth map キャッシュをクリアする。
// キャラクター切り替え時などに呼ぶ（Phase 4.3 以降で使用予定）。
func InvalidateDepthCache() {
	depthCacheMu.Lock()
	depthCache = make(map[string]*image.Gray)
	depthCacheMu.Unlock()
}
