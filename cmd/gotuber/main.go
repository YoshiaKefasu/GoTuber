// Package main is the entry point of GoTuber.
//
// Phase 1.3: 透過ウィンドウ + クリックスルー + アトラス読み込み。
// カメラ・マイク・マウス追従は次フェーズ。
package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/YoshiaKefasu/GoTuber/internal/audio"
	"github.com/YoshiaKefasu/GoTuber/internal/blink"
	"github.com/YoshiaKefasu/GoTuber/internal/character"
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

	// マイクキャプチャ開始（失敗時は口パク無効で続行）
	mover, err := audio.NewMover()
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
	ebiten.SetWindowSize(640, 480)
	// ウィンドウ位置を画面中央寄りに明示 (デフォルト (0,0) だとタスクバー裏に隠れる)
	ebiten.SetWindowPosition(200, 200)

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

	// Tweaks パネル
	tweaksState := tweaks.NewState()
	panel := tweaks.NewPanel(face, tweaksState, mover != nil)
	log.Printf("tweaks panel: F1 to toggle (audio checkbox: %s)", func() string {
		if mover != nil {
			return "enabled"
		}
		return "greyed (mic unavailable)"
	}())

	g := game.New(atlas, mouse.NewFollower(0.3), blink.New(), mover, panel, tweaksState)

	// ゲームループ
	// ebiten.Termination は kill switch 発火時の正常終了として扱う（終了コード 0）
	if err := ebiten.RunGameWithOptions(g, opts); err != nil && err != ebiten.Termination {
		log.Printf("GoTuber terminated with error: %v", err)
		os.Exit(1)
	}
}
