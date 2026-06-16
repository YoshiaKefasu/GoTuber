package audio

// Mover は Capture + EnvelopeFollower + MouthTracker を束ねる高レベル API。
// game.Update から Mover.Update() を呼ぶと最新の口パク状態が返る。
type Mover struct {
	capture  *Capture
	envelope *EnvelopeFollower
	mouth    *MouthTracker
}

// NewMover は Mover を初期化する。デバイスが見つからない場合はエラーを返す。
func NewMover() (*Mover, error) {
	c, err := NewCapture()
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
