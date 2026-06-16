// Package main is the entry point of GoTuber.
//
// Phase 1.3: 透過ウィンドウ + クリックスルー + アトラス読み込み。
// カメラ・マイク・マウス追従は次フェーズ。
package main

import (
	"log"
	"os"

	"github.com/YoshiaKefasu/GoTuber/internal/audio"
	"github.com/YoshiaKefasu/GoTuber/internal/blink"
	"github.com/YoshiaKefasu/GoTuber/internal/character"
	"github.com/YoshiaKefasu/GoTuber/internal/game"
	"github.com/YoshiaKefasu/GoTuber/internal/killswitch"
	"github.com/YoshiaKefasu/GoTuber/internal/mouse"
	"github.com/YoshiaKefasu/GoTuber/internal/tweaks"
	"github.com/hajimehoshi/ebiten/v2"
)

func main() {
	// キャラクター設定読み込み
	cfg, err := character.LoadConfig("config/default.yaml")
	if err != nil {
		log.Printf("failed to load config: %v", err)
		os.Exit(1)
	}
	log.Printf("loaded character: %s (base=%s, ext=%s)", cfg.Name, cfg.BasePath, cfg.Ext)

	// フェイルファスト: 設定妥当性検証（base_path / ext / 6 sheets / 150 images）
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

	// 透過背景 + クリックスルー
	//   - ScreenTransparent: RunGame 前
	//   - SetWindowMousePassthrough: Update 初回内 (Issue #3222 対策)
	opts := &ebiten.RunGameOptions{
		ScreenTransparent: true,
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
