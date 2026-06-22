//go:build camera

// Package camera は Phase 2 ZeroMQ IPC (MediaPipe サイドカー連携) を提供する。
//
// アーキテクチャ (docs/PHASE2.md §4.1):
//
//	GoTuber (Go)                        mp_server.py (Python)
//	┌──────────────────────────┐         ┌──────────────────┐
//	│ CameraTracker (PUB 5555) │───────> │ Face Landmarker  │
//	│ MPClient      (SUB 5556) │<─────── │ detection JSON   │
//	│ Mapper (Phase 2.4)       │         └──────────────────┘
//	└──────────────────────────┘
//
// Phase 2.2 スコープは CameraTracker のみ (frame publisher)。
// mpclient.go (Phase 2.3) と mapper.go (Phase 2.4) は別ファイルに分割予定。
//
// スレッド:
//   - CaptureLoop: 専用 goroutine (Ebitengine 60Hz Update ループと独立)
//   - State observer (SentCount / IsRunning / LastErrorAt): lock-free atomic
//   - Start / Close: sync.Mutex で直列化
//
// ビルドタグ: 本パッケージは `//go:build camera` でガード。
// Phase 1 ビルド (`go build ./cmd/gotuber`) には影響しない (Phase 2.0 確定事項)。
package camera

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"log"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/blackjack/webcam"
	"github.com/pebbe/zmq4"
)

// frameTopic は ZeroMQ PUB-SUB topic prefix 識別子。
// 現状 (Phase 2.3d): frame publish は prefix なし (Phase 2.2 実装、Phase 2.5+ で "frame " prefix 化予定)。
// Phase 2.3c で mp_server.py 側の detection topic prefix ("detection ") は確定済み、Go 側 mpclient.go (Phase 2.3b) と整合済み。
// 詳細: docs/PHASE2.md Section 4.3 通信プロトコル
//
// 現状 (Phase 2.3d 確定):
//   - mp_server.py (Phase 2.3c) は検出 publish 時に `"detection " + JSON` の形式で送信する。
//   - mpclient.go (Phase 2.3b) は `SetSubscribe("detection")` でフィルタ受信する。
//   - frame topic prefix (Go 側 publish) は Phase 2.5+ で対応予定 (現状は未着手で OK)。
//
// Phase 2.3c で mp_server.py 側 detection topic prefix 対応確定、Phase 2.5+ で frame topic prefix 化予定 (Go 側 CameraTracker):
//   - SendBytes 先頭に `frameTopic + " "` を付与:
//     pub.SendBytes([]byte(frameTopic+" "+string(payload)), 0)
//   - mp_server.py 側で `setsockopt(zmq.SUBSCRIBE, b"frame")` を設定
//   - これにより "frame" prefix のみ受信、複数 subscriber 対応
const frameTopic = "frame"

// CameraTracker は webcam capture → ZeroMQ publish の非同期ループを司る。
//
// 設計保証 (Phase 2.2 ユーザー要件):
//
//  1. 非同期 — CaptureLoop は専用 goroutine。Ebitengine 60Hz Update に影響なし。
//  2. クラッシュ安全 — panic は defer recover() で吸収、goroutine を graceful exit。
//  3. 再起動可能 — Close() → NewCameraTracker() → Start() のサイクルで何度でも
//     再起動可 (port bind 競合なし)。
//
// 所有権: Start 成功時にのみ device / pub / zmqCtx の所有権が tracker へ移る。
// error path は cleanupCtx フラグで defer 経由一括解放 (Phase 1.14.1 audio
// capture.go と同パターン)。リソース競合: 状態 observer は atomic、Start/Close は mutex。
type CameraTracker struct {
	// Config (immutable after NewCameraTracker)
	cameraID    int
	framePort   int
	width       int
	height      int
	jpegQuality int

	// State (lock-free via sync/atomic)
	sentCount   atomic.Int64
	lastErrorNS atomic.Int64 // 0 = no error yet
	running     atomic.Bool  // true while CaptureLoop is alive

	// Concurrency
	mu     sync.Mutex
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Resources: Start で確保 → captureLoop の defer releaseResources で解放。
	// 競合しない根拠: Start が mu 下で設定し mu 解放後に goroutine を起動、
	// releaseResources は goroutine 終了後に走る (concurrent reader は存在しない)。
	device *webcam.Webcam
	pub    *zmq4.Socket
	zmqCtx *zmq4.Context
}

// NewCameraTracker は tracker を生成する。リソースは Start() まで touch されない。
//
// デフォルト値: width ≤ 0 → 640、height ≤ 0 → 480、jpegQuality が範囲外 → 75。
//
// 引数: cameraID (/dev/videoN の N、Linux のみ。Phase 2.5+ で Windows DirectShow
// 対応予定)、framePort (ZeroMQ PUB bind port、Phase 2.2 は 5555 想定)、
// width/height (best effort。device 非対応時は Bounds() に従う)、jpegQuality (1..100)。
func NewCameraTracker(cameraID, framePort, width, height, jpegQuality int) *CameraTracker {
	if width <= 0 {
		width = 640
	}
	if height <= 0 {
		height = 480
	}
	if jpegQuality <= 0 || jpegQuality > 100 {
		jpegQuality = 75
	}
	return &CameraTracker{
		cameraID:    cameraID,
		framePort:   framePort,
		width:       width,
		height:      height,
		jpegQuality: jpegQuality,
	}
}

// Start はカメラと ZeroMQ socket を確保し CaptureLoop を別 goroutine で起動する。
//
// エラー時: defer クリーンアップで部分確保リソースを全解放 (Phase 1.14.1 audio
// lifecycle パターン踏襲)。失敗した tracker は破棄して NewCameraTracker で作り直す
// のが安全。冪等: 既に running なら no-op (re-check inside mutex で race 防止)。
func (t *CameraTracker) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	// mutex 内で再チェック (Phase 2.2 review S-1 修正: lock 外で Load すると
	// 並行 Start で device/zmqCtx/pub の取り合いと goroutine リークが発生)
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.running.Load() {
		return nil
	}

	var (
		device *webcam.Webcam
		zmqCtx *zmq4.Context
		pub    *zmq4.Socket
	)
	cleanupCtx := true
	defer func() {
		if !cleanupCtx {
			return
		}
		if pub != nil {
			_ = pub.Close()
		}
		if zmqCtx != nil {
			_ = zmqCtx.Term()
		}
		if device != nil {
			_ = device.Close()
		}
	}()

	var err error
	device, err = webcam.Open("/dev/video" + strconv.Itoa(t.cameraID))
	if err != nil {
		return fmt.Errorf("webcam.Open /dev/video%d: %w", t.cameraID, err)
	}
	// SetImageFormat は best effort: MJPEG 優先、なければ first available に
	// フォールバック (upstream blackjack/webcam examples/stdout_streamer/
	// stdout_streamer.go:64-83 パターン踏襲)。失敗しても Start は継続し、
	// device default を使う (Phase 1 軽量性重視。Phase 2.5+ で厳格化予定)。
	formats := device.GetSupportedFormats()
	var format webcam.PixelFormat
	for f, name := range formats {
		if strings.Contains(name, "MJPEG") || strings.Contains(name, "MJPG") {
			format = f
			break
		}
	}
	if format == 0 {
		for f := range formats {
			format = f
			break
		}
	}
	if format != 0 {
		_, _, _, err = device.SetImageFormat(format, uint32(t.width), uint32(t.height))
		if err != nil {
			log.Printf("camera: SetImageFormat(%v, %d, %d) failed: %v (device default kept)",
				format, t.width, t.height, err)
		}
	} else {
		log.Printf("camera: no supported formats from device (using kernel default)")
	}
	if err := device.StartStreaming(); err != nil {
		return fmt.Errorf("webcam.StartStreaming: %w", err)
	}

	zmqCtx, err = zmq4.NewContext()
	if err != nil {
		return fmt.Errorf("zmq4.NewContext: %w", err)
	}
	pub, err = zmqCtx.NewSocket(zmq4.PUB)
	if err != nil {
		return fmt.Errorf("zmq4.NewSocket(PUB): %w", err)
	}
	addr := fmt.Sprintf("tcp://*:%d", t.framePort)
	if err := pub.Bind(addr); err != nil {
		return fmt.Errorf("zmq4 PUB bind %s: %w", addr, err)
	}

	// 成功: 所有権を tracker へ移譲。defer は no-op になる。
	t.device = device
	t.pub = pub
	t.zmqCtx = zmqCtx
	cleanupCtx = false

	loopCtx, cancel := context.WithCancel(ctx)
	t.cancel = cancel
	t.wg.Add(1)
	t.running.Store(true)
	go t.captureLoop(loopCtx)

	log.Printf("camera: tracker started (device /dev/video%d, pub %s, %dx%d q=%d)",
		t.cameraID, addr, t.width, t.height, t.jpegQuality)
	return nil
}

// captureLoop は CaptureLoop 本体。1 フレーム読み → NRGBA 化 → JPEG → base64 →
// JSON → ZMQ PUB send を webcam native framerate で繰り返す。
//
// 終了条件: ctx.Done() (Close 経由) または panic (defer recover で吸収)。
// エラー: ログ + 100ms backoff で継続 (致命的エラーは trigger しない)。
//
// defer は LIFO 順で: recover → releaseResources → running.Store(false) → wg.Done。
// この順序で panic 時も全クリーンアップが保証される。
func (t *CameraTracker) captureLoop(ctx context.Context) {
	defer t.wg.Done()
	defer t.running.Store(false)
	defer t.releaseResources()

	defer func() {
		if r := recover(); r != nil {
			t.lastErrorNS.Store(time.Now().UnixNano())
			log.Printf("camera: tracker panic recovered (goroutine exiting): %v", r)
		}
	}()

	seq := uint64(0)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		frame, err := t.readFrame()
		if err != nil {
			if errors.Is(err, context.Canceled) || ctx.Err() != nil {
				return
			}
			t.lastErrorNS.Store(time.Now().UnixNano())
			log.Printf("camera: readFrame error (backoff 100ms): %v", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		payload, err := t.encodeFrame(frame, seq)
		if err != nil {
			t.lastErrorNS.Store(time.Now().UnixNano())
			log.Printf("camera: encodeFrame error: %v", err)
			continue
		}

		// TODO(Phase 2.5+): flag を zmq4.DONTWAIT (=1) に、SNDTIMEO を 100ms に。
		// 現状は flag=0 (blocking)。SNDHWM 1000 を超えると mp_server.py 側が
		// 詰まると Send がブロックする。Phase 2.2 は 1 subscriber 前提なので OK。
		// 注: zmq4 v1.4.0 の Socket.Send は string を受ける。[]byte payload は
		// SendBytes を使う (Phase 2.2 review bonus 修正: 実コンパイルエラー)。
		if _, err := t.pub.SendBytes(payload, 0); err != nil {
			t.lastErrorNS.Store(time.Now().UnixNano())
			log.Printf("camera: ZMQ send error: %v", err)
			continue
		}

		seq++
		t.sentCount.Add(1)
	}
}

// readFrame は webcam から 1 フレーム読み取り *image.NRGBA を返す。
//
// Phase 2.5+ 最適化候補: 既知 format (MJPEG / YUYV) 直接デコード、NV12 → GPU
// upload (Ebitengine texture 直接連携)、image/draw.Draw で per-pixel ループ置換。
// 現状の pixel loop は 640x480 で CPU 6-8% 程度 (MJPEG 30 FPS)。
func (t *CameraTracker) readFrame() (*image.NRGBA, error) {
	raw, err := t.device.ReadFrame()
	if err != nil {
		return nil, fmt.Errorf("webcam.ReadFrame: %w", err)
	}
	if len(raw) == 0 {
		return nil, errors.New("camera: empty webcam frame")
	}

	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("image.Decode: %w", err)
	}

	if nrgba, ok := img.(*image.NRGBA); ok {
		return nrgba, nil
	}

	// フォールバック: pixel-by-pixel で NRGBA へ変換。
	// TODO(Phase 2.5+): image/draw.Draw(out, b, img, b.Min, draw.Src) で置換。
	b := img.Bounds()
	out := image.NewNRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			out.SetNRGBA(x, y, color.NRGBA{
				R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(bl >> 8), A: 255,
			})
		}
	}
	return out, nil
}

// encodeFrame は NRGBA → JPEG (configurable quality) → base64 → JSON wrap。
//
// wire format (docs/PHASE2.md §4.3):
//
//	{"type":"frame","seq":N,"width":W,"height":H,"data":"<base64 JPEG>"}
//
// JSON 化の選択理由: tcpdump | jq で wire デバッグ可能 (Phase 2.10)。
// msgpack / protobuf 比で ~10% オーバーヘッド (Phase 2.5+ で再評価)。
func (t *CameraTracker) encodeFrame(img *image.NRGBA, seq uint64) ([]byte, error) {
	var jpegBuf bytes.Buffer
	if err := jpeg.Encode(&jpegBuf, img, &jpeg.Options{Quality: t.jpegQuality}); err != nil {
		return nil, fmt.Errorf("jpeg.Encode: %w", err)
	}

	fj := FrameJSON{
		Type:   frameTopic,
		Seq:    seq,
		Width:  img.Bounds().Dx(),
		Height: img.Bounds().Dy(),
		Data:   base64.StdEncoding.EncodeToString(jpegBuf.Bytes()),
	}
	return json.Marshal(fj)
}

// Close は CaptureLoop をキャンセルし、リソース解放を待ってから return。
//
// 任意の goroutine から複数回呼んで OK (cancel ガード + wg.Wait 冪等)。
// Start 前 / Start 失敗後でも安全 (nil ガード)。
//
// 推奨再起動パターン: tracker.Close() → NewCameraTracker(...) → newTracker.Start(ctx)。
// 同一インスタンス再 Start も可能だが、port bind 競合回避のため NewCameraTracker 推奨。
func (t *CameraTracker) Close() error {
	t.mu.Lock()
	if t.cancel != nil {
		t.cancel()
		t.cancel = nil
	}
	t.mu.Unlock()

	t.wg.Wait() // captureLoop の defer chain が走り切るまで待つ
	return nil
}

// releaseResources は device / pub / zmqCtx を解放する。
// captureLoop の defer から呼ばれる (goroutine exit 時に 1 回だけ実行)。
// nil ガード + 各 Close/Term の冪等性で Start 失敗 path でも安全に呼べる。
func (t *CameraTracker) releaseResources() {
	if t.pub != nil {
		_ = t.pub.Close()
		t.pub = nil
	}
	if t.zmqCtx != nil {
		_ = t.zmqCtx.Term()
		t.zmqCtx = nil
	}
	if t.device != nil {
		_ = t.device.Close()
		t.device = nil
	}
}

// --- 状態 observer (lock-free, 任意の goroutine から安全) ---

// SentCount は Start 後の publish 成功フレーム累計を返す。
func (t *CameraTracker) SentCount() int64 { return t.sentCount.Load() }

// IsRunning は CaptureLoop が生存している間 true。
// Close 後・panic 後の graceful exit 後は false。
func (t *CameraTracker) IsRunning() bool { return t.running.Load() }

// LastErrorAt は直近エラーの発生時刻を返す。
// エラー未発生時はゼロ値 time.Time{} (IsZero() == true)。
// Phase 2.8 で Tweaks パネルの status 表示に使う予定。
func (t *CameraTracker) LastErrorAt() time.Time {
	ns := t.lastErrorNS.Load()
	if ns == 0 {
		return time.Time{}
	}
	return time.Unix(0, ns)
}
