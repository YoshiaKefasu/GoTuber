// Package main is the entry point of GoTuber.
//
// Phase 1.3: 透過ウィンドウ + クリックスルー + アトラス読み込み。
// カメラ・マイク・マウス追従は次フェーズ。
package main

import (
	"log"
	"os"

	"github.com/YoshiaKefasu/GoTuber/internal/character"
	"github.com/YoshiaKefasu/GoTuber/internal/game"
	"github.com/YoshiaKefasu/GoTuber/internal/killswitch"
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

	// ウィンドウ設定
	ebiten.SetWindowTitle(game.WindowTitle())
	ebiten.SetWindowSize(640, 480)

	// 透過背景 + クリックスルー
	//   - ScreenTransparent: RunGame 前
	//   - SetWindowMousePassthrough: Update 初回内 (Issue #3222 対策)
	opts := &ebiten.RunGameOptions{
		ScreenTransparent: true,
	}

	g := game.New(atlas)

	// ゲームループ
	// ebiten.Termination は kill switch 発火時の正常終了として扱う（終了コード 0）
	if err := ebiten.RunGameWithOptions(g, opts); err != nil && err != ebiten.Termination {
		log.Printf("GoTuber terminated with error: %v", err)
		os.Exit(1)
	}
}
