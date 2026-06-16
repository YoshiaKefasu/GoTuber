package audio

import (
	"encoding/binary"
	"fmt"
	"math"
	"sync"
	"sync/atomic"

	"github.com/gen2brain/malgo"
)

const (
	sampleRate = 48000
	channels   = 1
	format     = malgo.FormatS16
)

// Capture は malgo でマイクから PCM を受信し、各バッファの RMS を atomic に保存する。
//
// RMS は [0, 1] の正規化値（int16 を 32768 で割った RMS）。
// GetRMS は audio スレッドとの同期に sync/atomic を使う。
type Capture struct {
	ctx     *malgo.AllocatedContext
	device  *malgo.Device
	rmsBits uint64 // atomic, float64 bits
}

// NewCapture は malgo context + capture device を初期化する。
// デバイスが見つからない場合エラーを返す。
func NewCapture() (*Capture, error) {
	// backends=nil でデフォルトバックエンド自動選択
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("malgo init: %w", err)
	}

	c := &Capture{ctx: ctx}

	deviceConfig := malgo.DefaultDeviceConfig(malgo.Capture)
	deviceConfig.Capture.Format = format
	deviceConfig.Capture.Channels = channels
	deviceConfig.SampleRate = sampleRate

	// DataProc は duplex 用シグネチャ。Capture のみ使用時は pOutput は無視、pInput に PCM データ。
	onData := func(_, pInput []byte, _ uint32) {
		samples := decodePCM16(pInput)
		rms := computeRMS(samples)
		atomic.StoreUint64(&c.rmsBits, math.Float64bits(rms))
		// サンプル slice をプールに戻す (GC 圧削減)
		releasePCMSamples(samples)
	}

	device, err := malgo.InitDevice(ctx.Context, deviceConfig, malgo.DeviceCallbacks{
		Data: onData,
	})
	if err != nil {
		ctx.Free()
		return nil, fmt.Errorf("malgo init device: %w", err)
	}
	c.device = device
	return c, nil
}

// Start はマイクキャプチャを開始する。
func (c *Capture) Start() error {
	return c.device.Start()
}

// Stop はマイクキャプチャを停止し、device と context を解放する。
func (c *Capture) Stop() {
	if c.device != nil {
		_ = c.device.Stop() // cleanup: error は致命的ではない
		c.device.Uninit()
		c.device = nil
	}
	if c.ctx != nil {
		c.ctx.Uninit()
		c.ctx.Free()
		c.ctx = nil
	}
}

// GetRMS は最新の RMS（[0, 1]）を返す。
// audio スレッドが書き込む前に呼ばれた場合は 0 を返す。
func (c *Capture) GetRMS() float64 {
	bits := atomic.LoadUint64(&c.rmsBits)
	return math.Float64frombits(bits)
}

// pcmSamplePool は decodePCM16 の []int16 slice をプールして GC 圧を削減する。
// malgo コールバックは ~47 Hz (48kHz / 1024 frame) で発火するため、毎回 make すると GC が頻発する。
var pcmSamplePool = sync.Pool{
	New: func() any {
		s := make([]int16, 0, 1024)
		return &s
	},
}

// decodePCM16 はリトルエンディアン int16 PCM バイト列をサンプル配列に変換する。
// モノラル (channels=1) 専用。ステレオ入力が必要な場合は per-channel 処理を追加すること。
// 内部で sync.Pool を使い、[]int16 の割当を抑える。
func decodePCM16(data []byte) []int16 {
	n := len(data) / 2
	pooled := pcmSamplePool.Get().(*[]int16)
	if cap(*pooled) < n {
		// 容量不足なら新規スライス (古いものは GC)
		s := make([]int16, n)
		*pooled = s
	} else {
		*pooled = (*pooled)[:n]
	}
	samples := *pooled
	for i := range samples {
		samples[i] = int16(binary.LittleEndian.Uint16(data[i*2 : i*2+2]))
	}
	// 使用後はプールに戻す (computeRMS で使い終わったら呼ぶ)
	return samples
}

// releasePCMSamples は decodePCM16 で取得したスライスをプールに戻す。
// RMS 計算後に必ず呼ぶこと。
func releasePCMSamples(samples []int16) {
	if samples == nil {
		return
	}
	pooled := samples[:0]
	pcmSamplePool.Put(&pooled)
}

// computeRMS は int16 サンプル列の RMS を [0, 1] で返す。
func computeRMS(samples []int16) float64 {
	if len(samples) == 0 {
		return 0
	}
	var sum float64
	for _, s := range samples {
		v := float64(s) / 32768.0
		sum += v * v
	}
	return math.Sqrt(sum / float64(len(samples)))
}
