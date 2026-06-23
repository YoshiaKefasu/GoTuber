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

	// Camera 状態 (Phase 2.8 で追加、表示専用、TOML 永続化なし)。
	// ResetToDefaults の対象外 (Supervisor API から毎フレーム更新される runtime 表示専用 state)。
	CameraMode        string // "Mouse" / "Active" / "Lost Signal" / "Down"
	CameraRestartable bool   // Down 状態時のみ true、Restart ボタン有効化用

	// UI 表示
	PanelVisible bool // F1 で toggle

	// Dirty は Tweaks パネルで未保存の変更があるかどうか。
	// Phase 1.14.16: 明示的 Save ボタン方式 (自動 Save しない) の dirty flag。
	//   - ChangedHandler (slider/checkbox) で true 化
	//   - Save 成功 (TOML 書き出し完了) で false 化
	//   - ApplyTo (起動時ロード) では変化しない (state は fresh)
	//
	// Quit / X ボタン押下時に true のままでも構わない (Save 押してない変更は破棄するのが仕様、
	// docs/PHASE1.md Section 15.2 参照)。
	Dirty bool

	// Quit ボタンが押された
	QuitRequested bool
}

// NewState はデフォルト値で State を作成する。
//
// Phase 1.14.16: Dirty=false 初期化 (Save 押してない変更なしの状態)。
func NewState() *State {
	return &State{
		MouseResponsiveness: 0.3,
		BlinkEnabled:        true,
		AudioEnabled:        true,
		// Phase 1.14.15: 15.0x → 10.0x に下げる (詳細は PHASE1.md Section 13.7 参照)。
		AudioSensitivity:  10.0,
		CameraMode:        "Mouse",
		CameraRestartable: false,
		PanelVisible:      false,
		Dirty:             false,
	}
}

// ResetToDefaults は 4 つの永続化対象フィールドをデフォルト値に復元する (Phase 1.14.16 → Round 3 で YAGNI 削除)。
//
// 注意: このメソッドは Phase 1.14.16 Round 2 まで Reset ボタンから呼ばれていたが、
// Round 3 で Reset ボタン自体が YAGNI 削除されたため、現バージョンでは呼ばれない。
// 後方互換性のためメソッド自体は残しておく (将来別 Phase で必要になる可能性)。
// CameraMode / CameraRestartable は runtime 表示専用なので変更しない。
func (s *State) ResetToDefaults() {
	s.MouseResponsiveness = 0.3
	s.BlinkEnabled = true
	s.AudioEnabled = true
	s.AudioSensitivity = 10.0
}
