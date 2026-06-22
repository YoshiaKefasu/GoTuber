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
// Phase 2.3 スコープは MPClient (detection subscriber) のみ。
// capture.go (Phase 2.2) と対になる受信側。mapper.go (Phase 2.4) と
// supervisor.go (Phase 2.5) は別ファイルに分割予定。
//
// スレッド:
//   - ReceiveLoop: 専用 goroutine (呼び出し側が `go client.ReceiveLoop(ctx)` で起動)
//   - State observer (RecvCount / IsRunning / LastErrorAt): lock-free atomic
//   - NewMPClient / Close: sync.Mutex で直列化
//
// ビルドタグ: 本ファイルは `//go:build camera` でガード。
package camera

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/pebbe/zmq4"
)

// detectionTopic は ZMQ SUB subscribe filter。docs/PHASE2.md 4.3 で "detection"
// タイプの JSON を受信する想定。
//
// Phase 2.3c で確定: tools/mp_server.py は `pub.send_string(DETECTION_TOPIC + " " + json.dumps(msg))`
// で wire prefix 付き publish する。Go 側 SetSubscribe("detection") と 1:1 で整合済み
// (B 方針)。JSON 内の `type` フィールドは将来複数 type publish 対応の冗長防御。
//
// Wire format: "detection <JSON>" (single space separator)
// 参考: docs/PHASE2.md Section 4.3 通信プロトコル。
const detectionTopic = "detection"

// recvTimeout は ZMQ SUB RecvBytes のタイムアウト値。EAGAIN で graceful loop を継続し、
// loop 先頭の ctx.Done() チェックで shutdown 反応を可能にする。100ms は Ebitengine
// 60Hz Update ループ (16.6ms) より十分長く、graceful shutdown を 100ms 以内に保証する
// バランス (短すぎると CPU 浪費、長すぎると shutdown 反応遅延)。
const recvTimeout = 100 * time.Millisecond

// DetectionJSON は mp_server.py (Phase 2.1) が port 5556 に publish する
// detection result message の wire format。docs/PHASE2.md 4.3 の JSON スキーマ:
//
//	{"type":"detection","seq":N,"timestamp":T,"face_detected":B,
//	 "yaw":F,"pitch":F,"roll":F,"ear_left":F,"ear_right":F,
//	 "face_center_x":F,"face_center_y":F}
//
// 全 11 フィールド (PHASE2.md 4.3 仕様) を 1:1 で struct tag にマップ。
// timestamp は Unix epoch 秒の float64 (サブ秒精度、float64 精度劣化あり)。
type DetectionJSON struct {
	Type         string  `json:"type"`          // 固定値 "detection"
	Seq          uint64  `json:"seq"`           // 検出連番 (monotonic increase)
	Timestamp    float64 `json:"timestamp"`     // Unix epoch 秒 (float64)
	FaceDetected bool    `json:"face_detected"` // 顔未検出時 false + 残り全 0 埋め
	Yaw          float64 `json:"yaw"`           // 度 (-90..90 想定)
	Pitch        float64 `json:"pitch"`         // 度 (-45..45 想定)
	Roll         float64 `json:"roll"`          // 度 (-180..180 想定)
	EarLeft      float64 `json:"ear_left"`      // Eye Aspect Ratio (左目)
	EarRight     float64 `json:"ear_right"`     // Eye Aspect Ratio (右目)
	FaceCenterX  float64 `json:"face_center_x"` // 正規化 X 座標 (-1..1)
	FaceCenterY  float64 `json:"face_center_y"` // 正規化 Y 座標 (-1..1)
}

// DetectionResult は MPClient.Latest() が返す内部表現。wire 形式 (DetectionJSON)
// と mapper 層 (Phase 2.4) のインターフェースを分離する役割。
//
// YAGNI: Phase 2.3 では JSON デコード値をそのまま保持。yaw/pitch の -90..90 クランプや
// EAR ヒステリシスは mapper 層 (Phase 2.4 / 2.6) で処理。
//
// Timestamp は秒精度 (Phase 2.3 簡略化)。float64 → int64 秒変換でサブ秒は切り捨て。
// ログ・デバッグ用途で十分、フレーム同期は Seq で扱う。
type DetectionResult struct {
	FaceDetected bool      // デコード済み bool
	Timestamp    time.Time // Unix epoch 秒 (秒精度、UTC)
	Yaw          float64   // 度 (Phase 2.4 mapper が col 0..4 に変換)
	Pitch        float64   // 度 (Phase 2.4 mapper が row 0..4 に変換)
	Roll         float64   // 度 (Phase 2.3 は未使用、Phase 2.5+ で利用検討)
	EarLeft      float64   // 0..0.5 想定、瞬き検出のベース値
	EarRight     float64
	FaceCenterX  float64 // -1..1 正規化座標
	FaceCenterY  float64
}

// MPClient は mp_server.py (Phase 2.1) から port 5556 で detection JSON を受信する。
//
// 設計保証 (Phase 2.3 ユーザー要件):
//
//  1. シンプル受信 — SUB 1 本、最新 1 件のみ保持。channel buffer は不要 (mutex + struct)。
//  2. クラッシュ安全 — panic は defer recover() で吸収、goroutine を graceful exit。
//  3. 再起動可能 — Close() → NewMPClient() → ReceiveLoop() のサイクルで何度でも再起動可。
//
// 所有権: NewMPClient 成功時にのみ sub / zmqCtx の所有権が client へ移る。
// error path は cleanupCtx フラグで defer 経由一括解放 (Phase 1.14.1 audio capture.go
// と同パターン、Phase 2.2 capture.go と厳密一致)。
//
// ライフサイクル: NewMPClient (即時 bind) → go ReceiveLoop(ctx) (呼び出し側が起動) →
// Close (cancel + wg.Wait)。capture.go の Start/Close パターンと異なり、goroutine 起動は
// 呼び出し側の責任 (Phase 2.5 supervisor loop がラップ予定)。
//
// Phase 2.5+ への TODO:
//   - L3 supervisor loop (supervisor.go) でラップ、IsRunning() == false を検知したら
//     NewMPClient → ReceiveLoop サイクルで自動再起動 (配信中可用性方針 Section 1.1)。
//   - 顔未検出 1 秒タイマーは mapper 層 (Phase 2.4) で処理、MPClient はタイムスタンプ保持のみ。
//   - Tweaks パネル Camera Status 4 状態表示は Phase 2.8 で実装。
type MPClient struct {
	// Config (immutable after NewMPClient)
	detectionPort int

	// State (lock-free via sync/atomic)
	recvCount   atomic.Int64
	lastErrorNS atomic.Int64 // 0 = no error yet
	running     atomic.Bool  // true while ReceiveLoop is alive

	// Concurrency
	mu     sync.Mutex
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Latest snapshot (mu で保護、Latest() で読み出し)
	latest    DetectionResult
	latestSeq uint64
	latestOK  bool

	// Resources: NewMPClient で確保 → ReceiveLoop の defer releaseResources で解放。
	sub    *zmq4.Socket
	zmqCtx *zmq4.Context
}

// NewMPClient は SUB socket を port 5556 に bind し、MPClient を生成する。
//
// 起動フロー:
//  1. zmq4.NewContext — ZMQ context 生成
//  2. zmqCtx.NewSocket(SUB) — SUB socket 作成
//  3. SetSubscribe("detection") — topic filter (Phase 2.5+ で mp_server.py 側 prefix 対応)
//  4. SetRcvtimeo(100ms) — RecvBytes timeout、EAGAIN で graceful loop 継続
//  5. Bind("tcp://localhost:port") — localhost 限定 bind (Phase 2.7 セキュリティ)
//
// 引数: detectionPort (mp_server.py の publish port、Phase 2.3 は 5556 想定)
//
// 注: capture.go (Phase 2.2) と異なり、Start を分けず即時 bind する。SUB は接続待ちが
// default 動作なので、bind した時点で mp_server.py の publish 開始を待機できる。
// goroutine 起動は呼び出し側が `go client.ReceiveLoop(ctx)` で行う。
func NewMPClient(detectionPort int) (*MPClient, error) {
	var (
		zmqCtx *zmq4.Context
		sub    *zmq4.Socket
	)
	cleanupCtx := true
	defer func() {
		if !cleanupCtx {
			return
		}
		if sub != nil {
			_ = sub.Close()
		}
		if zmqCtx != nil {
			_ = zmqCtx.Term()
		}
	}()

	var err error
	zmqCtx, err = zmq4.NewContext()
	if err != nil {
		return nil, fmt.Errorf("zmq4.NewContext: %w", err)
	}
	sub, err = zmqCtx.NewSocket(zmq4.SUB)
	if err != nil {
		return nil, fmt.Errorf("zmq4.NewSocket(SUB): %w", err)
	}
	if err := sub.SetSubscribe(detectionTopic); err != nil {
		return nil, fmt.Errorf("zmq4 SUB SetSubscribe(%q): %w", detectionTopic, err)
	}
	if err := sub.SetRcvtimeo(recvTimeout); err != nil {
		return nil, fmt.Errorf("zmq4 SUB SetRcvtimeo(%v): %w", recvTimeout, err)
	}
	addr := fmt.Sprintf("tcp://localhost:%d", detectionPort)
	if err := sub.Bind(addr); err != nil {
		return nil, fmt.Errorf("zmq4 SUB bind %s: %w", addr, err)
	}

	log.Printf("camera: MPClient socket bound (sub %s, filter=%q)", addr, detectionTopic)
	c := &MPClient{
		detectionPort: detectionPort,
	}
	c.sub = sub
	c.zmqCtx = zmqCtx
	cleanupCtx = false
	return c, nil
}

// ReceiveLoop は goroutine で起動する受信ループ本体。ctx.Done() で graceful 終了。
//
// 呼び出し側は `go client.ReceiveLoop(ctx)` で起動する (Phase 2.5 supervisor loop が
// 管理する想定)。再起動防止: 既に running なら即 return (race-free、capture.go と同パターン)。
//
// フロー:
//  1. sub.RecvBytes(0) — blocking、ただし SetRcvtimeo 100ms 設定でタイムアウト時 EAGAIN
//  2. EAGAIN (timeout) → silent continue (loop 先頭で ctx.Done() チェック)
//  3. JSON parse → DetectionJSON → validateDetectionJSON → DetectionResult 変換
//  4. mu.Lock → latest/latestSeq/latestOK 更新 → mu.Unlock
//  5. recvCount.Add(1)
//
// エラーハンドリング:
//   - RecvBytes timeout (EAGAIN) → silent continue (normal flow)
//   - loopCtx cancel 中の error → graceful return
//   - その他の RecvBytes error → log + lastErrorNS 更新 + 100ms backoff + continue
//   - JSON parse 失敗 → log warning + lastErrorNS 更新 + continue (1 件 drop、次を待つ)
//   - type フィールド不正 → log warning + continue (将来 mp_server が複数 type publish 用)
//
// 終了条件: ctx.Done() または panic (defer recover で吸収)。
//
// defer は LIFO 順で: recover → releaseResources → running.Store(false) → wg.Done。
// この順序で panic 時も全クリーンアップが保証される (capture.go と同じ)。
func (c *MPClient) ReceiveLoop(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	// mutex 内で再チェック (capture.go review S-1 修正と同パターン: 並行 ReceiveLoop
	// 呼び出しで sub/zmqCtx の取り合いと goroutine リークを防止)
	c.mu.Lock()
	if c.running.Load() {
		c.mu.Unlock()
		return
	}
	loopCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	c.wg.Add(1)
	c.running.Store(true)
	c.mu.Unlock()

	defer c.wg.Done()
	defer c.running.Store(false)
	defer c.releaseResources()

	defer func() {
		if r := recover(); r != nil {
			c.lastErrorNS.Store(time.Now().UnixNano())
			log.Printf("camera: MPClient panic recovered (goroutine exiting): %v", r)
		}
	}()

	for {
		select {
		case <-loopCtx.Done():
			return
		default:
		}

		msg, err := c.sub.RecvBytes(0)
		if err != nil {
			// timeout (EAGAIN) は normal flow、silent continue
			if zmq4.AsErrno(err) == zmq4.Errno(syscall.EAGAIN) {
				continue
			}
			// loopCtx が cancel 済みなら graceful exit
			if loopCtx.Err() != nil {
				return
			}
			c.lastErrorNS.Store(time.Now().UnixNano())
			log.Printf("camera: MPClient RecvBytes error: %v", err)
			time.Sleep(recvTimeout)
			continue
		}

		var dj DetectionJSON
		if err := json.Unmarshal(msg, &dj); err != nil {
			c.lastErrorNS.Store(time.Now().UnixNano())
			log.Printf("camera: detection JSON parse error: %v (raw=%q)", err, truncateLog(msg, 100))
			continue
		}
		if err := validateDetectionJSON(&dj); err != nil {
			log.Printf("camera: detection validation failed: %v (dropped)", err)
			continue
		}

		dr := DetectionResult{
			FaceDetected: dj.FaceDetected,
			Timestamp:    time.Unix(int64(dj.Timestamp), 0).UTC(),
			Yaw:          dj.Yaw,
			Pitch:        dj.Pitch,
			Roll:         dj.Roll,
			EarLeft:      dj.EarLeft,
			EarRight:     dj.EarRight,
			FaceCenterX:  dj.FaceCenterX,
			FaceCenterY:  dj.FaceCenterY,
		}
		c.mu.Lock()
		c.latest = dr
		c.latestSeq = dj.Seq
		c.latestOK = true
		c.mu.Unlock()
		c.recvCount.Add(1)
	}
}

// validateDetectionJSON は DetectionJSON の必須フィールド (type) を検証する。
//
// Phase 2.3 では type == "detection" のみ受理。将来 mp_server.py が複数 type (例:
// "stats", "debug") を publish する場合に備えて早期 drop する (誤った型を mapper に
// 渡さない)。受信統計は recvCount には加算しない (1 件 drop)。
func validateDetectionJSON(dj *DetectionJSON) error {
	if dj.Type != "detection" {
		return fmt.Errorf("unsupported type %q (want \"detection\")", dj.Type)
	}
	return nil
}

// Latest は MPClient の最新検出結果を返す (non-blocking, mutex 保護)。
//
// 戻り値:
//   - dr: 最新検出結果 (顔未検出時は yaw/pitch/EAR 全 0、Phase 2.4 mapper が扱う)
//   - seq: 検出連番 (Phase 2.4 mapper の stale 検知に使える、フレーム突合用)
//   - ok: 一度も JSON を受信していない場合 false、1 件以上受信後 true
//
// Phase 2.4 mapper は 60Hz で呼ぶ想定。mutex は struct snapshot の短時間なので安全。
func (c *MPClient) Latest() (DetectionResult, uint64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.latest, c.latestSeq, c.latestOK
}

// Close は ReceiveLoop をキャンセルし、リソース解放を待ってから return。
//
// 任意の goroutine から複数回呼んで OK (cancel ガード + wg.Wait 冪等)。
// ReceiveLoop 未起動でも安全 (cancel == nil ガード、wg.Wait 即 return)。
//
// 推奨再起動パターン: client.Close() → NewMPClient(...) → newClient.ReceiveLoop(ctx)。
// 同一インスタンス再 ReceiveLoop も可能だが、port bind 競合回避のため NewMPClient 推奨。
func (c *MPClient) Close() error {
	c.mu.Lock()
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	c.mu.Unlock()

	c.wg.Wait() // ReceiveLoop の defer chain が走り切るまで待つ
	return nil
}

// releaseResources は sub / zmqCtx を解放する。
// ReceiveLoop の defer から呼ばれる (goroutine exit 時に 1 回だけ実行)。
// nil ガード + 各 Close/Term の冪等性で error path でも安全に呼べる (capture.go と同じ)。
func (c *MPClient) releaseResources() {
	if c.sub != nil {
		_ = c.sub.Close()
		c.sub = nil
	}
	if c.zmqCtx != nil {
		_ = c.zmqCtx.Term()
		c.zmqCtx = nil
	}
}

// --- 状態 observer (lock-free, 任意の goroutine から安全) ---

// RecvCount は ReceiveLoop 開始後の parse + validate 成功検出数累計を返す。
// capture.go の SentCount (Phase 2.2 PUB 送信数) と対称。
func (c *MPClient) RecvCount() int64 { return c.recvCount.Load() }

// IsRunning は ReceiveLoop が生存している間 true。
// Close 後・panic 後の graceful exit 後は false。Phase 2.5 supervisor loop が
// false 検知で NewMPClient → ReceiveLoop 再起動サイクルに使う予定。
func (c *MPClient) IsRunning() bool { return c.running.Load() }

// LastErrorAt は直近エラーの発生時刻を返す。
// エラー未発生時はゼロ値 time.Time{} (IsZero() == true)。
// Phase 2.8 で Tweaks パネルの status 表示に使う予定。
func (c *MPClient) LastErrorAt() time.Time {
	ns := c.lastErrorNS.Load()
	if ns == 0 {
		return time.Time{}
	}
	return time.Unix(0, ns)
}

// truncateLog は log 用に長いバイト列を切り詰める。ZMQ 受信失敗時のデバッグ用。
// len(b) <= max の場合はそのまま返す。
func truncateLog(b []byte, max int) string {
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "...(truncated)"
}
