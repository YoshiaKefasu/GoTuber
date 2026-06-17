package audio

import (
	"encoding/binary"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"unsafe"

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

// newContext は malgo デフォルトバックエンドで Alloc を初期化する。
// devices.go (ListDevices) と capture.go (NewCapture/NewCaptureByID) で共有。
func newContext() (*malgo.AllocatedContext, error) {
	return malgo.InitContext(nil, malgo.ContextConfig{}, nil)
}

// NewCapture は malgo context + capture device を初期化する。
// OS デフォルト入力デバイスを使用する。
// デバイスが見つからない場合エラーを返す。
func NewCapture() (*Capture, error) {
	return NewCaptureByID("")
}

// NewCaptureByID は指定した malgo 内部 device ID でキャプチャデバイスを初期化する。
// deviceID が空文字 "" の場合は OS デフォルト入力デバイスを使う。
//
// Phase 1.13a: ユーザーが Tweaks パネルで選択したマイクデバイスで起動するため。
// ID 不一致の場合はエラーを返し、呼び出し側は OS デフォルトにフォールバックする。
//
// S-2 修正: defer 化で LIFO 順序 (Uninit → Free) を保証。
// 旧コードは 3 つのエラー path (list devices / not found / init device) で
// 個別に ctx.Free() を呼んでいたが、ctx.Uninit() をスキップしており
// malgo godoc の "Free must only be called for an uninitialized context." に違反していた。
// defer に統一することで全ての return path で正しい順序が保証される。
//
// Phase 1.14.1 (audio lifecycle fix): defer の Uninit/Free は **失敗 path のみ** 実行
// するように変更。成功 path では ctx をそのまま Capture に持たせ、解放責任を
// Capture.Stop() に委ねる。
//
// 旧コードの致命的バグ:
//   - 成功時に defer で ctx が Uninit/Free 済みでも c.ctx はそのまま指し続ける
//   - その後 Capture.Stop() が再度 c.ctx.Uninit() / c.ctx.Free() を呼ぶ
//   - → 既に Free 済み ctx への double-free でヒープ破壊 / プロセス終了
//
// 実機で観測された症状:
//   F1 押下 → Tweaks パネル → ListComboButton 初期選択イベント
//   → onDeviceSelected("") → Mover.Restart("") → m.capture.Stop() (旧 capture)
//   → 上記 double-free で即座にクラッシュ。
//   ログ `config saved: audio.device_id = ""` の直後で app 終了。
//
// 修正方針:
//   - cleanupCtx フラグで success/error を分岐
//   - success path: cleanupCtx = false にして defer の cleanup を skip
//   - error path: cleanupCtx = true のまま、defer で Uninit → Free
//   - InitDevice 成功後は Capture.Stop() が device と context 両方を解放
func NewCaptureByID(deviceID string) (*Capture, error) {
	ctx, err := newContext()
	if err != nil {
		return nil, fmt.Errorf("malgo init: %w", err)
	}
	// ctx 解放責任の分岐フラグ。
	// true  = 失敗 path → defer で ctx を解放
	// false = 成功 path → defer を no-op にして Capture に所有権を移譲
	cleanupCtx := true
	defer func() {
		if cleanupCtx {
			// 失敗 path: LIFO 順 (Uninit → Free) を維持。
			// malgo godoc: "Free must only be called for an uninitialized context."
			// Uninit() は error を返す可能性があるが、defer 経路なので破棄 (cleanup 失敗で
			// プロセスを落とすと元も子もない。リークは許容)。
			_ = ctx.Uninit()
			ctx.Free()
		}
	}()

	deviceConfig := malgo.DefaultDeviceConfig(malgo.Capture)
	deviceConfig.Capture.Format = format
	deviceConfig.Capture.Channels = channels
	deviceConfig.SampleRate = sampleRate

	// deviceID 指定がある場合: ListDevices() で一致する DeviceInfo を探して pointer を設定
	if deviceID != "" {
		infos, err := ctx.Context.Devices(malgo.Capture)
		if err != nil {
			return nil, fmt.Errorf("malgo list devices: %w", err)
		}
		var matched bool
		for _, info := range infos {
			if info.ID.String() == deviceID {
				deviceConfig.Capture.DeviceID = unsafe.Pointer(info.ID.Pointer())
				matched = true
				break
			}
		}
		if !matched {
			return nil, fmt.Errorf("audio: device id %q not found", deviceID)
		}
	}
	// deviceID == "" → デフォルト (malgo.DeviceID ゼロ値 = OS 選択)

	// DataProc は duplex 用シグネチャ。Capture のみ使用時は pOutput は無視、pInput に PCM データ。
	// S-4: クロージャで Capture を参照。c.deviceID フィールドは削除済み (dead storage)。
	c := &Capture{ctx: ctx}
	device, err := malgo.InitDevice(ctx.Context, deviceConfig, malgo.DeviceCallbacks{
		Data: func(_, pInput []byte, _ uint32) {
			samples := decodePCM16(pInput)
			rms := computeRMS(samples)
			atomic.StoreUint64(&c.rmsBits, math.Float64bits(rms))
			// サンプル slice をプールに戻す (GC 圧削減)
			releasePCMSamples(samples)
		},
	})
	if err != nil {
		return nil, fmt.Errorf("malgo init device: %w", err)
	}
	c.device = device

	// 成功 path: ctx 解放責任を Capture に移譲。
	// defer は cleanupCtx==false なので何もしない。Capture.Stop() が
	// device.Uninit → ctx.Uninit → ctx.Free の順で解放する。
	// (※ 現状 InitDevice 後に error path はないため device cleanup は不要)
	cleanupCtx = false
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
