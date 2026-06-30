package audio

// Phase 1.14.14: adaptive noise gate の定数。固定閾値 (thresholdMouth0=0.05,
// thresholdMouth1=0.20) はそのまま残し、その手前で「環境ノイズ vs 声」を分離する
// ゲートを設ける。固定閾値を雑に下げると常時ノイズで口がパクパクするため、
// noise floor を毎フレーム追従させ、gate の open/close にヒステリシスを持たせる。
//
// 数値は実機 tweak 経由で確定 (2026-06-30)。部屋ノイズ RMS 0.01-0.02 環境で
// gate が過敏に開く問題を修正した。
//   - defaultMicSensitivity: ゲイン倍率。raw 0.005 程度でも 0.07 (MouthHalf) を超えるよう
//     10x を初期値に。sensitivity 0 のときはこれで自動初期化。
//   - minMicSensitivity / maxMicSensitivity: Tweaks slider のクランプ範囲。
//   - noiseFloorRiseRate: 環境ノイズ上昇時の追従速度 (1 更新あたり)。ゆっくり。
//   - noiseFloorFallRate: 環境ノイズ下降時の追従速度 (1 更新あたり)。上昇より速め。
//   - noiseFloorWarmupFrames: 起動直後の小さい常時ノイズで gate が即開くのを防ぐ
//     学習フレーム数。60fps 前提で約 1 秒。
//   - gateOpenMargin / gateCloseMargin: gate 開閉ヒステリシス (RMS 空間)。
//     open > close にすることで、gate 境界でフリッカしない。
//     0.008 / 0.004 は部屋ノイズ 0.01-0.02 で安定運用するための値。
//   - lowFloorGuardThreshold: noiseFloor がこの値未満のとき、floor はまだ
//     環境ノイズを学習中とみなす。この間は voiceAbsoluteThreshold 以上の
//     raw でなければ gate を開かない。
//   - voiceAbsoluteThreshold: floor 未学習時の absolute 閾値。部屋ノイズ
//     (0.01-0.02) がこの値未満であるため、この閾値で分離できる。
const (
	minMicSensitivity      = 1.0
	defaultMicSensitivity  = 10.0
	maxMicSensitivity      = 20.0
	noiseFloorRiseRate     = 0.02
	noiseFloorFallRate     = 0.08
	noiseFloorWarmupFrames = 60
	gateOpenMargin         = 0.008
	gateCloseMargin        = 0.004

	// low-floor guard: noiseFloor がこの値未満のとき floor 未学習とみなす。
	// 部屋ノイズ 0.01-0.02 環境で、warmup 後も floor が 0 に近い場合に
	// gate が即開くのを防ぐ。
	lowFloorGuardThreshold = 0.01

	// voiceAbsoluteThreshold: floor 未学習時の絶対開門閾値。
	// raw がこの値以上でなければ gate を開かない (margin 判定の前に適用)。
	// 部屋ノイズ (0.01-0.02) はこの閾値未満のため、ゲートが開かない。
	voiceAbsoluteThreshold = 0.03
)

// Mover は Capture + EnvelopeFollower + MouthTracker を束ねる高レベル API。
// game.Update から UpdateWithMetrics() を呼ぶと、最新の口パク状態と診断値が返る。
//
// Phase 1.14.14: noiseFloor / gateOpen / sensitivity を追加し、raw RMS を
// applyNoiseGate() で「環境ノイズ分を引いて gain」してから envelope へ流す。
// gate が閉じている間は envelope は 0 にクランプされるため、envelope の
// attack/release は「gate が開いている区間」だけ機能する。
type Mover struct {
	capture  *Capture
	envelope *EnvelopeFollower
	mouth    *MouthTracker

	// Phase 1.14.14: adaptive noise gate 状態。ゼロ値で初期化される。
	// noiseFloor は単調増加の exponential filter で raw RMS に追従。
	// gateOpen は現在の gate 状態 (true = voice 検出中)。
	// sensitivity はゲイン倍率 (0 のときは defaultMicSensitivity を自動セット)。
	// noiseWarmupFrames は gate を開く前に floor を学習したフレーム数。
	noiseFloor        float64
	gateOpen          bool
	sensitivity       float64
	noiseWarmupFrames int
}

// Metrics は UpdateWithMetrics() の戻り値。Tweaks パネルでの debug 表示と、
// game.Update から state.Audio* への伝播に使う。
//
// Phase 1.14.14: RMS / NoiseFloor / GatedRMS / Envelope / Mouth / GateOpen の
// 6 フィールド。呼び出し側が「無音なのか、ゲートで切られているのか、ゲートは
// 開いているがエンベロープで平滑化されているのか」を切り分けられる。
type Metrics struct {
	RMS        float64 // 生の RMS (gate 通過前)
	NoiseFloor float64 // 現在の adaptive noise floor
	GatedRMS   float64 // gate 通過 + gain 適用後の値 (envelope 入力)
	Envelope   float64 // envelope 平滑化後の値 (MouthTracker 入力)
	Mouth      int     // 口パク状態 (MouthClosed=0 / MouthHalf=1 / MouthOpen=2)
	GateOpen   bool    // noise gate が現在開いているか
}

// NewMover は Mover を初期化する。OS デフォルト入力デバイスを使う。
// デバイスが見つからない場合はエラーを返す。
//
// Phase 1.13a: 後方互換のため維持。新規コードは NewMoverByID を推奨。
func NewMover() (*Mover, error) {
	return NewMoverByID("")
}

// NewMoverByID は指定した malgo 内部 device ID で Mover を初期化する。
// deviceID が空文字 "" の場合は OS デフォルト入力デバイスを使う。
// デバイスが見つからない場合エラーを返す。
func NewMoverByID(deviceID string) (*Mover, error) {
	c, err := NewCaptureByID(deviceID)
	if err != nil {
		return nil, err
	}
	return &Mover{
		capture:  c,
		envelope: NewEnvelopeFollower(),
		mouth:    NewMouthTracker(),
	}, nil
}

// Start はマイクキャプチャを開始する。
func (m *Mover) Start() error {
	return m.capture.Start()
}

// Stop はマイクキャプチャを停止し、リソースを解放する。
func (m *Mover) Stop() {
	m.capture.Stop()
}

// UpdateWithMetrics は口パク更新に使った全 diagnostic 値を返す。
//
// Phase 1.14.14: 戻り値を Metrics 構造体に変更。Tweaks パネルの debug 表示と、
// game.Update から state.Audio* への伝播に使う。
//
// 処理順:
//  1. capture.GetRMS() で raw RMS 取得
//  2. applyNoiseGate(raw) で adaptive noise floor 追跡 + gate hysteresis + gain
//  3. envelope.Update(gated) で attack/release 平滑化
//  4. mouth.Update(envelope) でヒステリシス付き 3 状態口パクへマップ
//
// 既存呼び出しは game.go のみ (Phase 1.14.13 以前は (rms, env, mouth) tuple を
// 返していた)。Phase 1.14.14 で API 変更したが呼び出し側の修正は局所。
func (m *Mover) UpdateWithMetrics() Metrics {
	rawRMS := m.capture.GetRMS()
	gatedRMS := m.applyNoiseGate(rawRMS)
	envelope := m.envelope.Update(gatedRMS)
	mouth := m.mouth.Update(envelope)
	return Metrics{
		RMS:        rawRMS,
		NoiseFloor: m.noiseFloor,
		GatedRMS:   gatedRMS,
		Envelope:   envelope,
		Mouth:      mouth,
		GateOpen:   m.gateOpen,
	}
}

// applyNoiseGate は adaptive noise floor + gate hysteresis + gain を適用する。
//
// アルゴリズム (シンプル、YAGNI):
//
//  1. noise floor 追従: gate が閉じていて、かつ raw が open threshold を超えて
//     いない時だけ、raw RMS に向けて exponential filter で追従する
//     (rise 0.02 / fall 0.08 per update)。gate 開放中は floor を凍結し、
//     voice を noise として誤検出しない。
//     起動直後は floor=0 のため、常時ノイズ raw=0.0038 でも floor+openMargin を
//     超えて gate が即開いてしまう。そこで最初の 60 frame は gate を強制 closed にして
//     floor を先に学習させる。
//
//  2. gate ヒステリシス: open/close の閾値にマージン差 (open 0.008 / close 0.004)
//     を持たせ、境界でフリッカしない。raw > floor+open で開、raw < floor+close
//     で閉じる。
//
//     low-floor guard: noiseFloor が lowFloorGuardThreshold (0.01) 未満のとき、
//     floor はまだ環境ノイズを学習中とみなす。この間は raw が
//     voiceAbsoluteThreshold (0.03) 以上でなければ gate を開かない。
//     部屋ノイズ 0.01-0.02 はこの閾値未満のため、floor 学習中にゲートが
//     開くことを防ぐ。floor が 0.01 に到達すれば、通常の margin 判定に移行。
//
//  3. gain: gate 開放中は (raw - floor - closeMargin) * sensitivity を返す。
//     sensitivity 0 のときは defaultMicSensitivity (10) を採用する。
//     Tweaks の Mic Sensitivity slider から 1.0x..20.0x で調整できる。
//
// 戻り値:
//   - gate closed: 0
//   - gate open: 0..1 にクランプしたゲイン済み voice RMS
//
// 状態遷移: sensitivity / noiseFloor / gateOpen / noiseWarmupFrames はすべて *m に保存され、
// 次回呼び出しで参照される。
func (m *Mover) applyNoiseGate(raw float64) float64 {
	if m.sensitivity == 0 {
		m.sensitivity = defaultMicSensitivity
	}

	// 起動直後は floor が 0 から育つ前に gate が開くと、常時ノイズを voice と誤認する。
	// 最初の約 1 秒は gate を開かず、環境ノイズだけを学習する。
	if m.noiseWarmupFrames < noiseFloorWarmupFrames {
		m.updateNoiseFloor(raw)
		m.noiseWarmupFrames++
		m.gateOpen = false
		return 0
	}

	// 2) gate ヒステリシス判定。
	// gate closed 中に raw が open threshold を超えた場合は、まず voice とみなし、
	// その loud sample を noise floor に混ぜない。
	//
	// low-floor guard: noiseFloor が lowFloorGuardThreshold 未満のとき、
	// floor はまだ環境ノイズを学習中とみなす。この間は raw が
	// voiceAbsoluteThreshold 以上でなければ gate を開かない。
	// 部屋ノイズ 0.01-0.02 でもこの閾値未満のため、ゲートが開かない。
	if m.gateOpen {
		if raw < m.noiseFloor+gateCloseMargin {
			m.gateOpen = false
		}
	} else if raw > m.noiseFloor+gateOpenMargin {
		if m.noiseFloor >= lowFloorGuardThreshold || raw >= voiceAbsoluteThreshold {
			m.gateOpen = true
		}
	}

	// 3) gate closed → 無音扱い。ここでだけ floor を追従させる。
	if !m.gateOpen {
		m.updateNoiseFloor(raw)
		return 0
	}

	// 4) gate open → ノイズ分を差し引いて gain 適用
	voice := raw - m.noiseFloor - gateCloseMargin
	if voice < 0 {
		voice = 0
	}
	scaled := voice * m.sensitivity
	if scaled > 1 {
		scaled = 1
	}
	return scaled
}

func (m *Mover) updateNoiseFloor(raw float64) {
	rate := noiseFloorRiseRate
	if raw < m.noiseFloor {
		rate = noiseFloorFallRate
	}
	m.noiseFloor += rate * (raw - m.noiseFloor)
}

// Restart はキャプチャを新しいデバイス ID で再起動する。
// Phase 1.13a: ユーザーが Tweaks パネルでマイクを変更したときに呼ばれる。
//
// Phase 1.14.1 (audio lifecycle fix): 順序を
//
//	旧: Stop 旧 capture → NewCaptureByID 新 → Start 新 → 差し替え
//	新: NewCaptureByID 新 → Start 新 → Stop 旧 capture → 差し替え
//
// に変更。理由:
//   - 旧順序は新 capture 初期化失敗時に旧 capture を失い、Mover 全体が無効化する
//   - 新順序は新 capture 失敗時に旧 capture を温存し、stale device で口パク継続可能
//   - 旧 capture 維持により、Restart が "no-op" 的に安全 (ユーザーは旧デバイスで
//     動作していることを UI で確認できる)
//
// 設計判断 (トレードオフ):
//   - 利点: 失敗時フォールバック。Mover は新デバイスエラーに動じない。
//   - リスク: 同一デバイスの二重 open を許さないバックエンド (WASAPI exclusive mode 等)
//     では、新 capture 作成が "device busy" で失敗する可能性がある。
//     その場合、ユーザーには「旧デバイスが継続中」であることを通知すべき。
//     Phase 1.14.1 では YAGNI によりこの抑止 UI は実装せず、stale 維持を優先する。
//
// 失敗時:
//   - NewCaptureByID 失敗 → 旧 capture 維持。エラー返却 (呼び出し側でログ/UI 通知)。
//   - c.Start() 失敗 → 新 capture を Stop() (defer 同等)、旧 capture 維持。エラー返却。
//
// 成功時: 内部 capture が新しいデバイスに差し替わる。GetRMS() は新デバイスから取得開始。
//
// nil safety: m.capture が nil になる経路は通常ない (NewMoverByID 失敗時は Mover 自体
// が返らない) が、防御的に nil guard を入れる。nil の場合は新 capture に単純置換。
func (m *Mover) Restart(deviceID string) error {
	// 1) 先に新 capture を作る。失敗時は旧 capture を維持してエラー返却。
	c, err := NewCaptureByID(deviceID)
	if err != nil {
		return err
	}
	// 2) 新 capture を起動。失敗時は新 capture を Stop() して旧 capture を維持。
	if err := c.Start(); err != nil {
		c.Stop()
		return err
	}
	// 3) 新 capture の動作開始を確認してから、旧 capture を解放 (nil guard 付き)。
	if m.capture != nil {
		m.capture.Stop()
	}
	// 4) 内部 capture を差し替え。
	m.capture = c
	return nil
}

// SetSensitivity はゲイン倍率を設定する。minMicSensitivity (1.0) /
// maxMicSensitivity (20.0) でクランプする。
//
// Phase 1.14.15: Tweaks パネルの Mic Sensitivity slider から呼ばれる。
// 毎フレーム game.Update() 開始時に state.AudioSensitivity を反映する設計。
// clamp は UI スライダーが範囲外になった場合の防御 (ebitenui の slider は
// min/max を尊重するので通常起きないが、config 復元などで範囲外が来る可能性)。
//
// 影響: 次の applyNoiseGate() 呼び出しから新しいゲインが使われる。noise floor や
// gate 状態はリセットしない (reactive なゲイン調整を想定)。
func (m *Mover) SetSensitivity(value float64) {
	if value < minMicSensitivity {
		value = minMicSensitivity
	}
	if value > maxMicSensitivity {
		value = maxMicSensitivity
	}
	m.sensitivity = value
}
