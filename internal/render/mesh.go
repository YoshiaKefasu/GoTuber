// Package render は Ebitengine の DrawTriangles ベースのメッシュレンダリングを提供する。
// Phase 4.1: 最小メッシュレンダラー（フラットメッシュ、depth map 未使用）
// Phase 4.2: depth-weighted elastic morph で拡張予定
package render

import (
	"image"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

const (
	// GridSize はメッシュの分割数 (PHASE4.md 仕様: 32×32 grid)
	GridSize = 32
	// VertexCount は一辺の頂点数 (GridSize + 1)
	VertexCount = GridSize + 1
)

// MeshGrid はメッシュの頂点とインデックスを保持する。
// Phase 4.2 で depth map による頂点変位を追加する際の拡張ポイント。
type MeshGrid struct {
	Vertices []ebiten.Vertex
	Indices  []uint16
}

// GenerateFlatMesh は画像を GridSize×GridSize のフラットメッシュとして生成する。
// 頂点位置は DrawImage と同じ見た目になるようスケーリング・オフセット済み。
//
// 引数:
//   - imgW, imgH: 元画像のピクセルサイズ
//   - screenW, screenH: 描画先スクリーンサイズ
//   - alpha: 全頂点に適用するアルファ値 (0.0〜1.0)
//
// 戻り値: 生成されたメッシュグリッド
func GenerateFlatMesh(imgW, imgH, screenW, screenH, alpha float64) *MeshGrid {
	vertices := make([]ebiten.Vertex, VertexCount*VertexCount)
	indices := make([]uint16, 0, GridSize*GridSize*6)

	// DrawImage と同じスケーリング計算
	scaleX := screenW / float64(imgW)
	scaleY := screenH / float64(imgH)
	scale := math.Min(scaleX, scaleY)
	scaledW := float64(imgW) * scale
	scaledH := float64(imgH) * scale
	ox := (screenW - scaledW) / 2
	oy := (screenH - scaledH) / 2

	// 頂点生成
	for y := 0; y <= GridSize; y++ {
		for x := 0; x <= GridSize; x++ {
			idx := y*VertexCount + x

			// UV座標 (0.0〜1.0)
			u := float32(x) / float32(GridSize)
			v := float32(y) / float32(GridSize)

			// 画面座標
			dstX := float32(ox + float64(x)*scaledW/float64(GridSize))
			dstY := float32(oy + float64(y)*scaledH/float64(GridSize))

			vertices[idx] = ebiten.Vertex{
				SrcX:   u * float32(imgW),
				SrcY:   v * float32(imgH),
				DstX:   dstX,
				DstY:   dstY,
				ColorR: 1,
				ColorG: 1,
				ColorB: 1,
				ColorA: float32(alpha),
			}
		}
	}

	// インデックス生成（2三角形 per セル）
	for y := 0; y < GridSize; y++ {
		for x := 0; x < GridSize; x++ {
		 topLeft := uint16(y*VertexCount + x)
			topRight := uint16(y*VertexCount + x + 1)
			bottomLeft := uint16((y+1)*VertexCount + x)
			bottomRight := uint16((y+1)*VertexCount + x + 1)

			// 1つ目の三角形 (topLeft, topRight, bottomLeft)
			indices = append(indices, topLeft, topRight, bottomLeft)
			// 2つ目の三角形 (topRight, bottomRight, bottomLeft)
			indices = append(indices, topRight, bottomRight, bottomLeft)
		}
	}

	return &MeshGrid{
		Vertices: vertices,
		Indices:  indices,
	}
}

// SetAlpha はメッシュ全体のアルファ値を変更する。
// Phase 4.0 の α ブレンド遷移で使用。
//
// 注意: cached mesh の頂点データを直接変更する。
// 現状は Ebitengine の Draw() 単一 goroutine から、DrawTriangles 直前にだけ
// 呼ぶ前提なので安全。将来 Phase 4.2 で頂点変位を入れる際は、必要に応じて
// per-draw copy へ切り替える。
func (m *MeshGrid) SetAlpha(alpha float32) {
	for i := range m.Vertices {
		m.Vertices[i].ColorA = alpha
	}
}

// DrawMesh はメッシュを screen に描画する。
//
// 引数:
//   - screen: 描画先
//   - img: テクスチャ画像
//   - mesh: 描画するメッシュ
func DrawMesh(screen *ebiten.Image, img *ebiten.Image, mesh *MeshGrid) {
	if img == nil || mesh == nil {
		return
	}
	screen.DrawTriangles(mesh.Vertices, mesh.Indices, img, &ebiten.DrawTrianglesOptions{
		Filter: ebiten.FilterLinear,
	})
}

// DrawMeshWithAlpha はメッシュを指定アルファで screen に描画する。
// SetAlpha + DrawMesh を組み合わせたヘルパー。
func DrawMeshWithAlpha(screen *ebiten.Image, img *ebiten.Image, mesh *MeshGrid, alpha float32) {
	mesh.SetAlpha(alpha)
	DrawMesh(screen, img, mesh)
}

// MeshBounds はメッシュの描画範囲を返す (テスト・デバッグ用)。
func MeshBounds(mesh *MeshGrid) image.Rectangle {
	if len(mesh.Vertices) == 0 {
		return image.Rectangle{}
	}
	minX, minY := float32(math.MaxFloat32), float32(math.MaxFloat32)
	maxX, maxY := float32(-math.MaxFloat32), float32(-math.MaxFloat32)
	for _, v := range mesh.Vertices {
		if v.DstX < minX {
			minX = v.DstX
		}
		if v.DstY < minY {
			minY = v.DstY
		}
		if v.DstX > maxX {
			maxX = v.DstX
		}
		if v.DstY > maxY {
			maxY = v.DstY
		}
	}
	return image.Rect(
		int(math.Floor(float64(minX))),
		int(math.Floor(float64(minY))),
		int(math.Ceil(float64(maxX))),
		int(math.Ceil(float64(maxY))),
	)
}
