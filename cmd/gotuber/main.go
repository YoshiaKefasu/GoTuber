// Package main is the entry point of GoTuber.
//
// Phase 1.3: 透過ウィンドウ + クリックスルー + アトラス読み込み。
// カメラ・マイク・マウス追従は次フェーズ。
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/YoshiaKefasu/GoTuber/internal/audio"
	"github.com/YoshiaKefasu/GoTuber/internal/blink"
	"github.com/YoshiaKefasu/GoTuber/internal/character"
	"github.com/YoshiaKefasu/GoTuber/internal/config"
	"github.com/YoshiaKefasu/GoTuber/internal/game"
	"github.com/YoshiaKefasu/GoTuber/internal/killswitch"
	"github.com/YoshiaKefasu/GoTuber/internal/mouse"
	"github.com/YoshiaKefasu/GoTuber/internal/tweaks"
	"github.com/hajimehoshi/ebiten/v2"
)

var (
	flagTopmost = flag.Bool("topmost", false, "Always-on-top window (default: off; OBS captures regardless of Z-order)")
	flagDebugBG = flag.Bool("debug-bg", false, "Disable ScreenTransparent (black background) for visual debugging")
)

func init() {
	// 作業ディレクトリをプロジェクトのルート (= EXE の親ディレクトリ) に変更。
	// ダブルクリック起動時 / タスクスケジューラ起動時に config/ assets/ への
	// 相対パスが解決できるようする。
	//
	// 例: bin/gotuber.exe → cwd = bin の親 (プロジェクトルート)
	//     gotuber.exe (プロジェクト直下) → cwd = プロジェクトルート
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		// 親ディレクトリに config/default.yaml があればそこへ、なければ exeDir のまま
		for _, candidate := range []string{filepath.Dir(exeDir), exeDir} {
			if _, err := os.Stat(filepath.Join(candidate, "config", "default.yaml")); err == nil {
				if err := os.Chdir(candidate); err != nil {
					log.Printf("cwd: chdir %s failed: %v", candidate, err)
				} else {
					log.Printf("cwd: %s", candidate)
				}
				return
			}
		}
		log.Printf("cwd: config/default.yaml not found near %s, using CWD %s", exeDir, exeDir)
		if err := os.Chdir(exeDir); err != nil {
			log.Printf("cwd: fallback chdir %s failed: %v", exeDir, err)
		}
	}
}

func main() {
	flag.Parse()

	// キャラクター設定読み込み
	cfg, err := character.LoadConfig("config/default.yaml")
	if err != nil {
		log.Printf("failed to load config: %v", err)
		os.Exit(1)
	}
	log.Printf("loaded character: basePath=%s, ext=%s, rows=%d, cols=%d", cfg.BasePath, cfg.Ext, cfg.Rows, cfg.Cols)
	log.Printf("config keys: basePath, ext, rows, cols, sheets.{eyesOpen,eyesClosed}.{close,half,open}")
	log.Printf("mouse Y-axis flip removed (matches tomari-guruguru app.jsx:62)")

	// フェイルファスト: 設定妥当性検証（basePath / ext / 6 sheets / 150 images）
	if err := cfg.Validate(); err != nil {
		log.Printf("invalid config: %v", err)
		os.Exit(1)
	}

	// アトラス作成 + 非同期ロード
	atlas := character.NewAtlas(cfg)
	go func() {
		if err := atlas.LoadAll(); err != nil {
			log.Printf("atlas load error: %v", err)
		} else {
			log.Printf("atlas loaded: 150 images (6 sheets × 5×5)")
		}
	}()

	// OS シグナルハンドラ
	killswitch.Install()

	// Phase 1.13a: ユーザー設定 (TOML) 読み込み
	userCfg, err := config.Load()
	if err != nil {
		log.Printf("config load failed (using defaults): %v", err)
		userCfg = &config.Config{}
	}
	if userCfg.Audio.DeviceID == "" {
		log.Printf("config loaded: audio.device_id = (OS default)")
	} else {
		log.Printf("config loaded: audio.device_id = %s", userCfg.Audio.DeviceID)
	}

	// Phase 1.14.16: Tweaks 永続化の起動時ロード。TOML の [tweaks] セクションから
	// 4 フィールドを state に上書き。ゼロ値 (= TOML 未設定) は skip。
	tweaksState := tweaks.NewState()
	userCfg.Tweaks.ApplyTo(tweaksState)

	// マイクキャプチャ開始（失敗時は口パク無効で続行）
	// Phase 1.13a: 設定のデバイス ID を malgo に渡す
	mover, err := audio.NewMoverByID(userCfg.Audio.DeviceID)
	if err != nil {
		log.Printf("audio init failed (continuing without mouth movement): %v", err)
		mover = nil
	} else if err := mover.Start(); err != nil {
		log.Printf("audio start failed (continuing without mouth movement): %v", err)
		mover.Stop()
		mover = nil
	} else {
		defer mover.Stop()
		log.Printf("audio started: 48kHz mono, mic-driven mouth movement active")
	}

	// ウィンドウ設定
	ebiten.SetWindowTitle(game.WindowTitle())
	ebiten.SetWindowSize(1280, 720)
	// ウィンドウ位置を画面中央寄りに明示 (デフォルト (0,0) だとタスクバー裏に隠れる)
	ebiten.SetWindowPosition(200, 200)
	// ウィンドウリサイズ有効 (Ebitengine v2: WindowResizingModeEnabled でドラッグリサイズ可能)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	log.Printf("window resizing: enabled")

	// Ebitengine v2: デフォルトで floating=false (他ウィンドウの後ろにいける)
	// --topmost フラグで明示的に ON にできる
	ebiten.SetWindowFloating(*flagTopmost)
	log.Printf("window floating (always-on-top): %v", *flagTopmost)

	// 透過背景 + クリックスルー
	//   - ScreenTransparent: RunGame 前
	//   - SetWindowMousePassthrough: Update 初回内 (Issue #3222 対策)
	screenTransparent := !*flagDebugBG
	if !screenTransparent {
		log.Printf("--debug-bg: 黒背景 fallback (透過 OFF)")
	}
	opts := &ebiten.RunGameOptions{
		ScreenTransparent: screenTransparent,
	}

	// フォントロード (Gen Interface JP Regular 6.1MB embedded)
	face := tweaks.LoadFontFace(16)
	log.Printf("font loaded: Gen Interface JP Regular (16px)")

	// Tweaks パネル (Phase 1.14.16: initialDeviceID を ComboBox 初期選択に渡す)
	panel := tweaks.NewPanel(face, tweaksState, mover != nil, userCfg.Audio.DeviceID)
	log.Printf("tweaks panel: F1 to toggle (audio checkbox: %s)", func() string {
		if mover != nil {
			return "enabled"
		}
		return "greyed (mic unavailable)"
	}())

	// Phase 1.14.7: Panel にデバイス選択コールバックを設定。
	// tweaks パッケージは audio 操作を直接行わず、main.go がオーケストレータになる。
	//
	// ComboBox 初期表示で onDeviceSelected("") が飛んでも no-op にするための guard。
	// F1 押下直後の `config saved: audio.device_id = ""` 抑止が主目的。
	// 同じ device ID 選択 (ユーザーが既に選択中のデバイスを再選択) も no-op。
	//
	// 順序設計 (Phase 1.14.6 の Mover.Restart 仕様と連携):
	//   1. 同一 device ID なら早期 return (save も restart も走らない)
	//   2. mover がある場合: Restart を先に試す
	//      - Restart 失敗 → 旧 capture 維持 (Phase 1.14.6 で保証) → config 更新しない
	//        → 旧: save 成功 → restart 失敗時に「config は新 deviceID、runtime は旧 device」
	//          の不整合 (stale) 状態を作っていた
	//        → 新: restart が真に新 device で動いたことを確認してから config を更新するので
	//          config と runtime が常に一致する
	//   3. mover がない場合 (起動時 audio 初期化失敗): restart できないので
	//      config だけ保存 (次回の起動で新 deviceID が反映される)
	panel.SetOnDeviceSelected(func(deviceID string) {
		// 1. 同一 device guard (F1 初期表示の "" 含む、ユーザーの再選択も no-op)
		//    ログは出さない: F1 押下のたびに 1 行出るのはノイズ。
		//    ComboBox 初期表示で必ず発火するため、起動毎に出るとログが埋まる。
		currentDeviceID := userCfg.Audio.DeviceID
		if deviceID == currentDeviceID {
			return
		}
		// 2. mover なし: restart 不可なので config だけ保存 (次回の起動で反映)
		if mover == nil {
			userCfg.Audio.DeviceID = deviceID
			if err := userCfg.Save(); err != nil {
				log.Printf("config save failed: %v", err)
			} else {
				log.Printf("config saved: audio.device_id = %q (audio mover unavailable, will apply on next launch)", deviceID)
			}
			return
		}
		// 3. Restart を先に試す (Phase 1.14.6: 失敗時は旧 capture 維持 → config 更新しない)
		if err := mover.Restart(deviceID); err != nil {
			log.Printf("audio restart failed (continuing with previous device): %v", err)
			return
		}
		// 4. Restart 成功 → config を更新 (runtime と config を一致させる)
		userCfg.Audio.DeviceID = deviceID
		if err := userCfg.Save(); err != nil {
			log.Printf("config save failed (audio is running on new device but config file is stale): %v", err)
		} else {
			log.Printf("config saved: audio.device_id = %q", deviceID)
		}
	})
	panel.SetOnRefreshDevices(func() []audio.Device {
		// 同期 ListDevices を実行 (Refresh ボタンは ebitenui メインスレッドから呼ばれるため OK)
		devices, err := audio.ListDevices()
		if err != nil {
			log.Printf("device refresh failed: %v", err)
			return nil
		}
		log.Printf("device refresh: %d devices", len(devices))
		return devices
	})

	// Phase 1.14.16: Save ボタン押下時のオーケストレーション。
	// main.go が config.Save + state.Dirty=false を担当。
	// panel 側はエラー時 status 文字列を内部で更新、成功時は呼び出し側で Dirty=false + "saved"。
	panel.SetOnSaveRequested(func() error {
		userCfg.Tweaks.CaptureFrom(tweaksState)
		if err := userCfg.Save(); err != nil {
			log.Printf("config save failed: %v", err)
			return err
		}
		log.Printf("config saved: tweaks.* (%d fields captured)", 4)
		return nil
	})

	// Phase 1.13a P-5: 起動時バックグラウンドで WASAPI 遅延初期化対策として
	// exponential backoff (即時 → 1s → 2s → 4s) でデバイス列挙。
	// 失敗時は OS デフォルトにフォールバック (panel 初期表示の "(OS default)" のみ)。
	//
	// S-1: ebitenui は goroutine safe ではないため、panel.SetDevices() を直接呼ばず
	// バッファ付き channel にメッセージを送信 → Game.Update 内の select で
	// メインスレッドに dispatch する。容量 1 で十分 (送信 1 回のみ)。
	devicesCh := make(chan game.DeviceListMessage, 1)
	go func() {
		devices, err := audio.ListDevicesWithRetry()
		if err != nil {
			log.Printf("device enumeration failed (using OS default): %v", err)
			return
		}
		log.Printf("device enumeration: %d devices", len(devices))
		devicesCh <- game.DeviceListMessage{Devices: devices}
	}()

	g := game.New(atlas, mouse.NewFollower(0.3), blink.New(), mover, panel, tweaksState, devicesCh)

	// Phase 2.5: camera supervisor 起動 (libzmq 利用可能時のみ動作)。
	// `//go:build camera` ガード下の init() で cameraHook が設定される。
	// Phase 1 ビルド (-tags なし): cameraHook は nil → runCameraHook は no-op、
	//                            既存のマウス追従のみが動作 (Phase 1 ビルド影響なし)。
	// Camera ビルド (-tags camera): init() で L1/L2/L3 supervisor を起動する hook が
	//                              設定され、60Hz で mouse ↔ camera 排他制御が走る。
	// 設計判断 (YAGNI): main.go 自体は build tag なしで 1 行追加のみ、
	// カメラ固有コードは camera_hook_camera.go (`//go:build camera`) に分離。
	// Go の build tag はファイル単位なので、main.go 内に直接 build tag 下の関数を
	// 呼ぶコードは書けない (Phase 1 ビルドで未定義シンボルエラーになる)。
	// Phase 2.5: camera hook 専用 context。RunGame 終了時に cancel し、
	// supervisor / tracker / mpclient goroutine を graceful shutdown させる。
	cameraCtx, cameraCancel := context.WithCancel(context.Background())
	defer cameraCancel()
	runCameraHook(cameraCtx, g)

	// ゲームループ
	// ebiten.Termination は kill switch 発火時の正常終了として扱う（終了コード 0）
	if err := ebiten.RunGameWithOptions(g, opts); err != nil && err != ebiten.Termination {
		log.Printf("GoTuber terminated with error: %v", err)
		os.Exit(1)
	}
}
