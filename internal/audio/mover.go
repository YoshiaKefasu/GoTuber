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

// Restart はキャプチャを停止し、新しいデバイス ID で再起動する。
// Phase 1.13a: ユーザーが Tweaks パネルでマイクを変更したときに呼ばれる。
//
// 失敗時:
//   - Stop 済み + 新規デバイス初期化失敗 → エラーを返し、Mover は無効状態
//     (Update は GetRMS()=0 → mouth=0 を返すので、口パクが止まるだけで安全)
//   - 呼び出し側はエラー時に UI で通知すべき
//
// 成功時: 内部 capture が新しいデバイスに差し替わる。GetRMS() は新デバイスから取得開始。
func (m *Mover) Restart(deviceID string) error {
	m.capture.Stop()
	c, err := NewCaptureByID(deviceID)
	if err != nil {
		return err
	}
	if err := c.Start(); err != nil {
		c.Stop()
		return err
	}
	m.capture = c
	return nil
}
