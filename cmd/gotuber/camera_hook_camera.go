//go:build camera

// Phase 2.5: camera supervisor の起動 hook (Camera ビルドでのみ動作)。
//
// このファイルは `//go:build camera` ガード下で、init() で cameraHook を設定する。
// camera_hook.go のラッパー経由で main() から呼ばれる。
//
// Phase 2.5 スコープ:
//
//   - L2 MPClient (Phase 2.3、internal/camera/mpclient.go) の起動
//   - L3 Supervisor (Phase 2.5、internal/camera/supervisor.go) の起動
//   - 60Hz で supervisor.Mode() を game.SetCameraMode() に反映する goroutine
//
// Phase 2.10: CameraTracker (webcam capture) 依存を除去。
// webcam capture は mp_server.py (Python sidecar) が担当。
// Go 側は mp_server.py の spawn/監視 + mpclient 検出結果受信に集中。
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
// TCP port は tools/mp_server.py の default と一致させる:
//   - detectionPort (SUB 5556): Python → Go detection JSON publish
//
// Phase 2.10: framePort / cameraID / width / height / jpegQuality を削除。
// webcam capture は mp_server.py が担当 (Go 側は frame publish をしない)。
const (
	detectionPort = 5556
)

// cameraModeUpdateInterval は game.SetCameraMode() の更新間隔。
//
// 60Hz (16ms) は Ebitengine Update ループ (16.6ms) とほぼ同期、supervisor loop の
// supervisorLoopInterval (16ms) とも整合。これにより supervisor.Mode() の変更が
// 最大 1 frame 以内に game.Update に反映される (見た目の遅延なし)。
const cameraModeUpdateInterval = 16 * time.Millisecond

// init は cameraHook に L2/L3 supervisor 起動 + mode 反映 goroutine を登録する。
//
// Go の init() は main() 実行前に自動的に呼ばれる。Phase 1 ビルドではこのファイル
// 全体が build tag で除外されるため init() は呼ばれず、cameraHook はゼロ値 nil のまま。
//
// フロー:
//
//  1. L2 MPClient 生成 (即時 bind) + 起動 (port bind 失敗は graceful degradation)
//  2. L3 Supervisor 生成 + Start (mpclient 起動失敗は graceful degradation)
//  3. mp_server.py spawn (supervisor が監視・再起動)
//  4. 60Hz で supervisor.Mode() → game.SetCameraMode() 反映 goroutine 起動
//
// 各ステップ失敗時: ログ + 部分解放 + mouse mode 永続 (graceful degradation)。
// 配信中可用性方針 (Section 1.1): Python サイドカー不在や OpenCV ロード失敗でも
// GoTuber 本体は正常起動し、Phase 1 マウスモードで動作継続。
//
// Phase 2.10: CameraTracker 生成を削除。webcam capture は mp_server.py が担当。
func init() {
	cameraHook = func(ctx context.Context, g *game.Game) <-chan struct{} {
		done := make(chan struct{})

		// L2 MPClient 起動 (port bind 失敗 → graceful degradation)
		mpclient, err := camera.NewMPClient(detectionPort)
		if err != nil {
			log.Printf("camera: NewMPClient failed (continuing without camera): %v", err)
			close(done)
			return done
		}

		// L3 Supervisor 起動 (mpclient 起動失敗 → graceful degradation)
		supervisor := camera.NewSupervisor(mpclient, nil)
		if err := supervisor.Start(ctx); err != nil {
			log.Printf("camera: supervisor.Start failed (continuing without camera): %v", err)
			mpclient.Close()
			close(done)
			return done
		}
		g.SetSupervisor(supervisor)
		// Phase 2.7: supervisor 起動後、mp_server.py を spawn (失敗しても mouse fallback で継続)。
		if err := supervisor.StartMPServer(); err != nil {
			log.Printf("camera: mp_server.py start failed (monitorMPServer will retry after 1s backoff): %v", err)
		}

		// 60Hz で supervisor.Mode() → game.SetCameraMode() 反映 goroutine。
		// 終了時 (ctx.Done): mouse mode (0) に戻して game に通知。
		// Phase 2.5.1: goroutine 終了時に done を close し、main() が cleanup 完了を待てるようにする。
		go func() {
			defer close(done)
			ticker := time.NewTicker(cameraModeUpdateInterval)
			defer ticker.Stop()
			defer supervisor.Stop()
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
		return done
	}
}
