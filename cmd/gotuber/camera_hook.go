// Phase 2.5: camera supervisor hook のための薄いラッパー。
//
// `//go:build camera` ガード下の init() で cameraHook が設定される。
// Phase 1 ビルドでは nil のまま → runCameraHook() は no-op。
//
// Go の build tag はファイル単位なので、main.go に直接 `//go:build camera` ガード下の
// 関数を呼ぶコードを書くと Phase 1 ビルドで未定義シンボルエラーになる。
// このラッパーパターンで Phase 1 ビルド影響ゼロ + Camera ビルドで hook 動作を実現。
//
// 設計判断 (YAGNI 厳守):
//   - cameraHook 変数を nil デフォルトにし、build tag 下で上書き
//   - runCameraHook() は nil チェックして呼ぶ安全ラッパー
//   - Phase 1 ビルド: cameraHook はゼロ値 (nil) → no-op
//   - Camera ビルド: cameraHook_camera.go の init() で hook 実装が代入される
package main

import (
	"context"

	"github.com/YoshiaKefasu/GoTuber/internal/game"
)

// cameraHookFunc は Phase 2.5 で camera supervisor を起動する hook のシグネチャ。
//
// 引数:
//
//	ctx — supervisor loop の lifecycle context (cancel で graceful shutdown)
//	g   — camera mode を反映する Game インスタンス (SetCameraMode() で毎フレーム更新)
//
// `//go:build camera` 下の init() で設定される。
// Phase 1 ビルドでは nil (= no-op)。
type cameraHookFunc func(ctx context.Context, g *game.Game)

// cameraHook は hook 実装への関数ポインタ。build tag 下で上書きされる。
//
// ゼロ値 (nil) がデフォルト = Phase 1 ビルド時の挙動。
// Camera ビルドでは init() で L1/L2/L3 supervisor を起動する関数に置き換わる。
var cameraHook cameraHookFunc

// runCameraHook は cameraHook を呼ぶ安全ラッパー。
//
// Phase 1 ビルド: nil チェックで即 return (no-op、Phase 1 ビルド影響ゼロ)。
// Camera ビルド: init() で設定された hook を実行 (L1/L2/L3 supervisor 起動)。
//
// 呼び出し側: main() 内の game.New() 直後・ebiten.RunGameWithOptions() 直前を想定。
// RunGameWithOptions は同期関数で戻らないため、hook は goroutine 起動する設計
// (camera_hook_camera.go の実装で go func() {...}() を発行)。
func runCameraHook(ctx context.Context, g *game.Game) {
	if cameraHook != nil {
		cameraHook(ctx, g)
	}
}
