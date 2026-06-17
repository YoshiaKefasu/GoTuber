package audio

// Mover は Capture + EnvelopeFollower + MouthTracker を束ねる高レベル API。
// game.Update から Mover.Update() を呼ぶと最新の口パク状態が返る。
type Mover struct {
	capture  *Capture
	envelope *EnvelopeFollower
	mouth    *MouthTracker
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

// Update は audio スレッドから最新の RMS を読み取り、
// エンベロープで平滑化し、口パク状態 (0/1/2) を返す。
// 毎フレーム (game.Update) から呼ぶ。
func (m *Mover) Update() int {
	rms := m.capture.GetRMS()
	env := m.envelope.Update(rms)
	return m.mouth.Update(env)
}

// Restart はキャプチャを新しいデバイス ID で再起動する。
// Phase 1.13a: ユーザーが Tweaks パネルでマイクを変更したときに呼ばれる。
//
// Phase 1.14.1 (audio lifecycle fix): 順序を
//   旧: Stop 旧 capture → NewCaptureByID 新 → Start 新 → 差し替え
//   新: NewCaptureByID 新 → Start 新 → Stop 旧 capture → 差し替え
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
