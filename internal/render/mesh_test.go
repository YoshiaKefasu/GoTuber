package render

import (
	"testing"
)

func TestGenerateFlatMesh(t *testing.T) {
	// テストケース: 1200x1200 画像を 1280x720 に描画
	imgW, imgH := 1200.0, 1200.0
	screenW, screenH := 1280.0, 720.0
	alpha := 1.0

	mesh := GenerateFlatMesh(imgW, imgH, screenW, screenH, alpha)

	// 頂点数チェック
	expectedVertices := VertexCount * VertexCount
	if len(mesh.Vertices) != expectedVertices {
		t.Errorf("vertices count: got %d, want %d", len(mesh.Vertices), expectedVertices)
	}

	// インデックス数チェック (32x32x2x3)
	expectedIndices := GridSize * GridSize * 6
	if len(mesh.Indices) != expectedIndices {
		t.Errorf("indices count: got %d, want %d", len(mesh.Indices), expectedIndices)
	}

	// アルファ値チェック
	for i, v := range mesh.Vertices {
		if v.ColorA != float32(alpha) {
			t.Errorf("vertex %d alpha: got %f, want %f", i, v.ColorA, alpha)
			break
		}
	}

	// SrcX/SrcY は画像ピクセル座標として [0, imgW] / [0, imgH] に収まる
	for i, v := range mesh.Vertices {
		if v.SrcX < 0 || v.SrcX > float32(imgW) {
			t.Errorf("vertex %d SrcX out of range: %f", i, v.SrcX)
			break
		}
		if v.SrcY < 0 || v.SrcY > float32(imgH) {
			t.Errorf("vertex %d SrcY out of range: %f", i, v.SrcY)
			break
		}
	}

	// インデックス範囲チェック
	for i, idx := range mesh.Indices {
		if int(idx) >= len(mesh.Vertices) {
			t.Errorf("index %d out of range: %d (max %d)", i, idx, len(mesh.Vertices)-1)
			break
		}
	}
}

func TestMeshBounds(t *testing.T) {
	mesh := GenerateFlatMesh(100, 100, 200, 200, 1.0)
	bounds := MeshBounds(mesh)

	// 200x200 ウィンドウに 100x100 画像 (スケール 2.0) を描画
	// 期待範囲: (0,0)-(200,200)
	if bounds.Dx() != 200 || bounds.Dy() != 200 {
		t.Errorf("bounds size: got %dx%d, want 200x200", bounds.Dx(), bounds.Dy())
	}
}

func TestSetAlpha(t *testing.T) {
	mesh := GenerateFlatMesh(100, 100, 200, 200, 1.0)

	// アルファを 0.5 に変更
	mesh.SetAlpha(0.5)

	for i, v := range mesh.Vertices {
		if v.ColorA != 0.5 {
			t.Errorf("vertex %d alpha after SetAlpha: got %f, want 0.5", i, v.ColorA)
			break
		}
	}
}

func TestMeshGridConsistency(t *testing.T) {
	// 異なる画像サイズでもメッシュが正しく生成されることを確認
	testCases := []struct {
		imgW, imgH, screenW, screenH float64
	}{
		{1200, 1200, 1280, 720},
		{800, 600, 1920, 1080},
		{100, 100, 100, 100},
	}

	for _, tc := range testCases {
		mesh := GenerateFlatMesh(tc.imgW, tc.imgH, tc.screenW, tc.screenH, 1.0)

		// 頂点数は常に一定
		if len(mesh.Vertices) != VertexCount*VertexCount {
			t.Errorf("imgW=%v imgH=%v: vertices count wrong", tc.imgW, tc.imgH)
		}

		// インデックス数は常に一定
		if len(mesh.Indices) != GridSize*GridSize*6 {
			t.Errorf("imgW=%v imgH=%v: indices count wrong", tc.imgW, tc.imgH)
		}

		// 全頂点の ColorR/G/B は 1.0 (白色)
		for i, v := range mesh.Vertices {
			if v.ColorR != 1.0 || v.ColorG != 1.0 || v.ColorB != 1.0 {
				t.Errorf("vertex %d color: R=%f G=%f B=%f", i, v.ColorR, v.ColorG, v.ColorB)
				break
			}
		}
	}
}
