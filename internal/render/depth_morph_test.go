package render

import (
	"image"
	"image/color"
	"math"
	"testing"
)

func TestSampleDepth_NilMap(t *testing.T) {
	// nil depth map → 0.0
	v := SampleDepth(nil, 0.5, 0.5)
	if v != 0.0 {
		t.Errorf("SampleDepth(nil) = %f, want 0.0", v)
	}
}

func TestSampleDepth_WhitePixel(t *testing.T) {
	// 1x1 white depth map → 1.0
	grey := image.NewGray(image.Rect(0, 0, 1, 1))
	grey.SetGray(0, 0, color.Gray{Y: 255})

	v := SampleDepth(grey, 0.5, 0.5)
	if v != 1.0 {
		t.Errorf("SampleDepth(white) = %f, want 1.0", v)
	}
}

func TestSampleDepth_BlackPixel(t *testing.T) {
	// 1x1 black depth map → 0.0
	grey := image.NewGray(image.Rect(0, 0, 1, 1))
	grey.SetGray(0, 0, color.Gray{Y: 0})

	v := SampleDepth(grey, 0.5, 0.5)
	if v != 0.0 {
		t.Errorf("SampleDepth(black) = %f, want 0.0", v)
	}
}

func TestSampleDepth_Gradient(t *testing.T) {
	// 2x2 depth map with known values
	grey := image.NewGray(image.Rect(0, 0, 2, 2))
	grey.SetGray(0, 0, color.Gray{Y: 0})   // (0,0) = 0
	grey.SetGray(1, 0, color.Gray{Y: 85})  // (1,0) ≈ 1/3
	grey.SetGray(0, 1, color.Gray{Y: 170}) // (0,1) ≈ 2/3
	grey.SetGray(1, 1, color.Gray{Y: 255}) // (1,1) = 1.0

	tests := []struct {
		u, v, want float64
	}{
		{0.0, 0.0, 0.0},
		{1.0, 0.0, 85.0 / 255.0},
		{0.0, 1.0, 170.0 / 255.0},
		{1.0, 1.0, 1.0},
	}

	for _, tc := range tests {
		got := SampleDepth(grey, tc.u, tc.v)
		if math.Abs(got-tc.want) > 0.01 {
			t.Errorf("SampleDepth(%.1f, %.1f) = %f, want %f", tc.u, tc.v, got, tc.want)
		}
	}
}

func TestSampleDepth_Clamping(t *testing.T) {
	// UV が 0.0〜1.0 範囲外でも clamped で値が返る
	grey := image.NewGray(image.Rect(0, 0, 2, 2))
	grey.SetGray(0, 0, color.Gray{Y: 100})

	// 負の UV → clamped to (0,0)
	v := SampleDepth(grey, -0.5, -0.5)
	if v != 100.0/255.0 {
		t.Errorf("SampleDepth(negative UV) = %f, want %f", v, 100.0/255.0)
	}

	// 1.0 超 UV → clamped to (1,1)
	grey.SetGray(1, 1, color.Gray{Y: 200})
	v = SampleDepth(grey, 1.5, 1.5)
	if v != 200.0/255.0 {
		t.Errorf("SampleDepth(over UV) = %f, want %f", v, 200.0/255.0)
	}
}

func TestEdgeWeight(t *testing.T) {
	tests := []struct {
		u, v, want float64
	}{
		{0.5, 0.5, 0.2}, // 中央 → 0.2 (最低重み保証, Phase 4.3 hotfix: 0.3→0.2)
		{0.0, 0.5, 1.0}, // 左端中央 → 1.0
		{0.5, 0.0, 1.0}, // 上端中央 → 1.0
		{0.0, 0.0, 1.0}, // 左上 → 1.0 (clamped)
		{1.0, 1.0, 1.0}, // 右下 → 1.0 (clamped)
	}

	for _, tc := range tests {
		got := edgeWeight(tc.u, tc.v)
		if math.Abs(got-tc.want) > 0.01 {
			t.Errorf("edgeWeight(%.1f, %.1f) = %f, want %f", tc.u, tc.v, got, tc.want)
		}
	}
}

func TestUpdateMorphElastic_ConvergeToTarget(t *testing.T) {
	// EMA は target に収束する
	e := &MorphElastic{ElX: 0, ElY: 0}
	targetX, targetY := 10.0, -5.0

	// 100 フレーム更新 → target に十分近づく
	for i := 0; i < 100; i++ {
		UpdateMorphElastic(e, targetX, targetY)
	}

	if math.Abs(e.ElX-targetX) > 0.01 {
		t.Errorf("ElX after 100 frames = %f, want ~%f", e.ElX, targetX)
	}
	if math.Abs(e.ElY-targetY) > 0.01 {
		t.Errorf("ElY after 100 frames = %f, want ~%f", e.ElY, targetY)
	}
}

func TestUpdateMorphElastic_ZeroTarget(t *testing.T) {
	// target=0 のとき、elastic は 0 に収束する
	e := &MorphElastic{ElX: 5.0, ElY: 3.0}

	for i := 0; i < 100; i++ {
		UpdateMorphElastic(e, 0, 0)
	}

	if math.Abs(e.ElX) > 0.01 {
		t.Errorf("ElX after converge to 0 = %f, want ~0", e.ElX)
	}
	if math.Abs(e.ElY) > 0.01 {
		t.Errorf("ElY after converge to 0 = %f, want ~0", e.ElY)
	}
}

func TestGenerateMorphedMesh_NilDepth(t *testing.T) {
	// depth map が nil → flat mesh と同じ
	flat := GenerateFlatMesh(1200, 1200, 1280, 720, 1.0)
	morphed := GenerateMorphedMesh(1200, 1200, 1280, 720, MorphParams{
		DepthMap: nil,
		ElX:     10,
		ElY:     10,
		Alpha:   1.0,
	})

	if len(flat.Vertices) != len(morphed.Vertices) {
		t.Errorf("vertex count mismatch: flat=%d morphed=%d", len(flat.Vertices), len(morphed.Vertices))
	}

	// 全頂点の座標が一致するか
	for i := range flat.Vertices {
		if flat.Vertices[i].DstX != morphed.Vertices[i].DstX {
			t.Errorf("vertex %d DstX mismatch: flat=%f morphed=%f", i, flat.Vertices[i].DstX, morphed.Vertices[i].DstX)
			break
		}
		if flat.Vertices[i].DstY != morphed.Vertices[i].DstY {
			t.Errorf("vertex %d DstY mismatch: flat=%f morphed=%f", i, flat.Vertices[i].DstY, morphed.Vertices[i].DstY)
			break
		}
	}
}

func TestGenerateMorphedMesh_UniformDepth(t *testing.T) {
	// 全白 depth map → 全頂点が均等に変位する
	grey := image.NewGray(image.Rect(0, 0, 1200, 1200))
	for y := 0; y < 1200; y++ {
		for x := 0; x < 1200; x++ {
			grey.SetGray(x, y, color.Gray{Y: 255})
		}
	}

	flat := GenerateFlatMesh(1200, 1200, 1280, 720, 1.0)
	morphed := GenerateMorphedMesh(1200, 1200, 1280, 720, MorphParams{
		DepthMap:  grey,
		ElX:      5.0, // 右に 5px
		ElY:      0,
		Alpha:    1.0,
		Strength: 8.0, // Phase 4.3: デフォルト Strength
	})

	// 中央の頂点 (u=0.5, v=0.5) は edgeWeight ≈ 0.3 (最低重み) なので
	// 小さな変位がある (~1.5px)。全頂点が完全に止まることはない。
	centerIdx := (GridSize/2)*VertexCount + GridSize/2
	centerDeltaX := float64(morphed.Vertices[centerIdx].DstX - flat.Vertices[centerIdx].DstX)
	if centerDeltaX > 2.0 {
		t.Errorf("center vertex DstX delta = %f, want < 2.0 (edgeWeight min=0.3 limits center displacement)", centerDeltaX)
	}

	// 端の頂点 (u=0, v=0.5) は edgeWeight = 1.0 なので変位が大きい
	edgeIdx := (GridSize / 2) * VertexCount // x=0, y=GridSize/2
	edgeDeltaX := float64(morphed.Vertices[edgeIdx].DstX - flat.Vertices[edgeIdx].DstX)
	if edgeDeltaX < 2.0 {
		t.Errorf("edge vertex DstX delta = %f, want > 2.0", edgeDeltaX)
	}
}

func TestDepthMapPath(t *testing.T) {
	path := DepthMapPath("assets/characters/_default", "A", 2, 3)
	want := "assets/characters/_default/A/depth/r2c3.png"
	if path != want {
		t.Errorf("DepthMapPath() = %q, want %q", path, want)
	}
}

// ─── Phase 4.5+: Transition warp テスト ────────────────────────────────────

func TestGenerateMorphedMesh_WarpOnly(t *testing.T) {
	// depth map なし、warp のみ → flat mesh に warp が適用される
	flat := GenerateFlatMesh(1200, 1200, 1280, 720, 1.0)
	warpX := 20.0 // 右に 20px warp
	morphed := GenerateMorphedMesh(1200, 1200, 1280, 720, MorphParams{
		DepthMap: nil,
		Alpha:    1.0,
		WarpX:    warpX,
		WarpY:    0,
	})

	// 頂点数は flat と同じ
	if len(flat.Vertices) != len(morphed.Vertices) {
		t.Errorf("vertex count mismatch: flat=%d morphed=%d", len(flat.Vertices), len(morphed.Vertices))
	}

	// 端の頂点 (u=0, v=0.5) は edgeWeight=1.0 なので warpX がそのまま適用される
	edgeIdx := (GridSize / 2) * VertexCount // x=0, y=GridSize/2
	deltaX := float64(morphed.Vertices[edgeIdx].DstX - flat.Vertices[edgeIdx].DstX)
	if math.Abs(deltaX-warpX) > 0.1 {
		t.Errorf("edge vertex DstX delta = %f, want ~%f (warpX)", deltaX, warpX)
	}
}

func TestGenerateMorphedMesh_WarpAndDepth(t *testing.T) {
	// depth map + warp の両方が適用されることを確認
	grey := image.NewGray(image.Rect(0, 0, 1200, 1200))
	for y := 0; y < 1200; y++ {
		for x := 0; x < 1200; x++ {
			grey.SetGray(x, y, color.Gray{Y: 255}) // 全白
		}
	}

	flat := GenerateFlatMesh(1200, 1200, 1280, 720, 1.0)
	warpX := 15.0
	morphed := GenerateMorphedMesh(1200, 1200, 1280, 720, MorphParams{
		DepthMap: grey,
		ElX:      5.0,
		ElY:      0,
		Alpha:    1.0,
		Strength: 8.0,
		WarpX:    warpX,
		WarpY:    0,
	})

	// 端の頂点: elastic morph + warp が両方適用される
	edgeIdx := (GridSize / 2) * VertexCount
	deltaX := float64(morphed.Vertices[edgeIdx].DstX - flat.Vertices[edgeIdx].DstX)

	// elastic morph の寄与: depth=1.0 * ElX=5.0 * strengthScale=0.8 * weight=1.0 = 4.0
	// warp の寄与: warpX=15.0 * weight=1.0 = 15.0
	// 合計: ~19.0
	if deltaX < 10.0 {
		t.Errorf("edge vertex DstX delta = %f, want > 10 (elastic + warp combined)", deltaX)
	}
}

func TestGenerateMorphedMesh_WarpDecaysWithEdgeWeight(t *testing.T) {
	// warp は edgeWeight に依存するため、中心より端が大きく変位する
	warpX := 20.0
	morphed := GenerateMorphedMesh(1200, 1200, 1280, 720, MorphParams{
		DepthMap: nil,
		Alpha:    1.0,
		WarpX:    warpX,
		WarpY:    0,
	})
	flat := GenerateFlatMesh(1200, 1200, 1280, 720, 1.0)

	// 中心頂点 (u=0.5, v=0.5): edgeWeight ≈ 0.2 → warpX * 0.2 ≈ 4.0
	centerIdx := (GridSize/2)*VertexCount + GridSize/2
	centerDelta := float64(morphed.Vertices[centerIdx].DstX - flat.Vertices[centerIdx].DstX)

	// 端頂点 (u=0, v=0.5): edgeWeight = 1.0 → warpX * 1.0 = 20.0
	edgeIdx := (GridSize / 2) * VertexCount
	edgeDelta := float64(morphed.Vertices[edgeIdx].DstX - flat.Vertices[edgeIdx].DstX)

	if edgeDelta <= centerDelta {
		t.Errorf("edge delta (%f) should be > center delta (%f)", edgeDelta, centerDelta)
	}
}
