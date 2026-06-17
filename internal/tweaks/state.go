package tweaks

// State は Tweaks パネルの現在の設定値。
// 単一 goroutine (game.Update) からのみ読み書き (mutex なし)。
type State struct {
	// Mouse responsiveness (0.05 〜 1.0、値が大きいほど追従が速い)
	MouseResponsiveness float64

	// Blink 自動まばたきを有効化
	BlinkEnabled bool

	// Audio 口パクを有効化 (false なら mouth=0 固定)
	AudioEnabled bool

	// Audio mic sensitivity (1.0..20.0、ゲイン倍率)。
	// Phase 1.14.15: Tweaks パネルの Mic Sensitivity slider から書き換え。
	// game.Update が毎フレーム audio.Mover.SetSensitivity(state.AudioSensitivity) を呼ぶ。
	// デフォルト 10.0x (Phase 1.14.14 終了時の 15.0x は無音ノイズで口が半開きになるため下げる)。
	AudioSensitivity float64

	// Audio debug values (Tweaks 表示用)。
	// game.Update が audio.Mover.UpdateWithMetrics() の結果を毎フレーム書く。
	// Phase 1.14.14: 4 フィールド追加。RMS は生値、Floor は adaptive noise gate の
	// 現在の floor 推定値、GatedRMS は gate 通過 + gain 適用後の値、GateOpen は
	// gate の状態 (true = voice 検出中)。Tweaks パネルで 2 行表示し、「無音なのか、
	// ゲートで切られているのか、ゲイン不足なのか」を切り分けられる。
	AudioRMS        float64
	AudioNoiseFloor float64
	AudioGatedRMS   float64
	AudioEnvelope   float64
	AudioMouthState int
	AudioGateOpen   bool

	// UI 表示
	PanelVisible bool // F1 で toggle

	// Quit ボタンが押された
	QuitRequested bool
}

// NewState はデフォルト値で State を作成する。
func NewState() *State {
	return &State{
		MouseResponsiveness: 0.3,
		BlinkEnabled:        true,
		AudioEnabled:        true,
		// Phase 1.14.15: 15.0x → 10.0x に下げる (詳細は PHASE1.md Section 13.7 参照)。
		AudioSensitivity:    10.0,
		PanelVisible:        false,
	}
}
