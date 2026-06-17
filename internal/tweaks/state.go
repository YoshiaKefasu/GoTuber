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

	// Audio debug values (Tweaks 表示用)。
	// game.Update が audio.Mover.UpdateWithMetrics() の結果を毎フレーム書く。
	AudioRMS        float64
	AudioEnvelope   float64
	AudioMouthState int

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
		PanelVisible:        false,
	}
}
