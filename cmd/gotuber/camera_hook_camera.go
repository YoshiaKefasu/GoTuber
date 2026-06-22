//go:build camera

// Phase 2.5: camera supervisor の起動 hook (Camera ビルドでのみ動作)。
//
// このファイルは `//go:build camera` ガード下で、init() で cameraHook を設定する。
// camera_hook.go のラッパー経由で main() から呼ばれる。
//
// Phase 2.5 スコープ:
//
//   - L1 CameraTracker (Phase 2.2、internal/camera/capture.go) の起動
//   - L2 MPClient (Phase 2.3、internal/camera/mpclient.go) の起動
//   - L3 Supervisor (Phase 2.5、internal/camera/supervisor.go) の起動
//   - 60Hz で supervisor.Mode() を game.SetCameraMode() に反映する goroutine
//
// Phase 2.5 スコープ外 (Phase 2.7 で実装):
//
//   - mp_server.py サブプロセス spawn・監視・再起動
//   - 5 回連続失敗時の Tweaks "Camera Down" 表示
//
// ビルドタグ: `//go:build camera` でガード。Phase 1 ビルドには影響しない
// (camera_hook.go の cameraHook はゼロ値 nil、runCameraHook が no-op)。
package main

import (
	"context"
	"log"
	"time"

	"github.com/YoshiaKefasu/GoTuber/internal/camera"
	"github.com/YoshiaKefasu/GoTuber/internal/game"
)

// camera 統合パラメータ (Phase 2.5 仕様、tools/mp_server.py と整合)。
//
// ZeroMQ port は tools/mp_server.py (Phase 2.1) の default と一致させる:
//   - framePort (PUB 5555): Go → Python frame publish
//   - detectionPort (SUB 5556): Python → Go detection JSON publish
//
// cameraID = 0 は /dev/video0 (Linux V4L2)。Phase 2.5+ で Windows DirectShow 対応予定。
// width/height は best effort、device 非対応時は webcam default に従う。
// jpegQuality = 75 はバランス (高 = 綺麗 / 低 = 軽い)、Phase 2.2 確定値。
const (
	framePort     = 5555
	detectionPort = 5556
	cameraID      = 0
	width         = 640
	height        = 480
	jpegQuality   = 75
)

// cameraModeUpdateInterval は game.SetCameraMode() の更新間隔。
//
// 60Hz (16ms) は Ebitengine Update ループ (16.6ms) とほぼ同期、supervisor loop の
// supervisorLoopInterval (16ms) とも整合。これにより supervisor.Mode() の変更が
// 最大 1 frame 以内に game.Update に反映される (見た目の遅延なし)。
const cameraModeUpdateInterval = 16 * time.Millisecond

// init は cameraHook に L1/L2/L3 supervisor 起動 + mode 反映 goroutine を登録する。
//
// Go の init() は main() 実行前に自動的に呼ばれる。Phase 1 ビルドではこのファイル
// 全体が build tag で除外されるため init() は呼ばれず、cameraHook はゼロ値 nil のまま。
//
// フロー:
//
//  1. L1 CameraTracker 生成 + Start (camera_open 失敗は graceful degradation)
//  2. L2 MPClient 生成 (即時 bind) + 起動 (port bind 失敗は graceful degradation)
//  3. L3 Supervisor 生成 + Start (tracker/mpclient 起動失敗は graceful degradation)
//  4. 60Hz で supervisor.Mode() → game.SetCameraMode() 反映 goroutine 起動
//
// 各ステップ失敗時: ログ + 部分解放 + mouse mode 永続 (graceful degradation)。
// 配信中可用性方針 (Section 1.1): Python サイドカー不在や OpenCV ロード失敗でも
// GoTuber 本体は正常起動し、Phase 1 マウスモードで動作継続。
func init() {
	cameraHook = func(ctx context.Context, g *game.Game) {
		// L1 CameraTracker 起動 (camera_open 失敗 → graceful degradation)
		tracker := camera.NewCameraTracker(cameraID, framePort, width, height, jpegQuality)
		if err := tracker.Start(ctx); err != nil {
			log.Printf("camera: tracker.Start failed (continuing without camera): %v", err)
			return
		}

		// L2 MPClient 起動 (port bind 失敗 → graceful degradation)
		mpclient, err := camera.NewMPClient(detectionPort)
		if err != nil {
			log.Printf("camera: NewMPClient failed (continuing without camera): %v", err)
			tracker.Close()
			return
		}

		// L3 Supervisor 起動 (tracker/mpclient 起動失敗 → graceful degradation)
		supervisor := camera.NewSupervisor(tracker, mpclient, nil)
		if err := supervisor.Start(ctx); err != nil {
			log.Printf("camera: supervisor.Start failed (continuing without camera): %v", err)
			tracker.Close()
			mpclient.Close()
			return
		}

		// 60Hz で supervisor.Mode() → game.SetCameraMode() 反映 goroutine。
		// 終了時 (ctx.Done): mouse mode (0) に戻して game に通知。
		go func() {
			ticker := time.NewTicker(cameraModeUpdateInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					g.SetCameraMode(0) // mouse mode に戻す
					return
				case <-ticker.C:
					g.SetCameraMode(int(supervisor.Mode()))
				}
			}
		}()
	}
}
