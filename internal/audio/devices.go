package audio

import (
	"fmt"
	"time"

	"github.com/gen2brain/malgo"
)

// Device はユーザーに表示する入力デバイス情報。
//
// ID は malgo 内部の一意 ID (DeviceID.String() で文字列化)。
// Name は OS から取得した表示名 ("USB Microphone" 等)。
//
// Phase 1.13a: ユーザー選択を TOML に保存するのは ID の方 (表示名重複対策)。
type Device struct {
	ID   string
	Name string
}

// ListDevices は malgo からシステム上の全入力デバイスを列挙する。
//
// 戻り値:
//   - 成功: デバイス一覧 (0 個の可能性あり、特に Windows 起動直後の WASAPI 遅延初期化時)
//   - 失敗: nil, error (malgo 初期化失敗 / Devices() 失敗)
//
// 注意: 起動直後は WASAPI の遅延初期化で空配列が返ることがある
// (PHASE1.md Section 10.4 R-1 / P-5 参照)。リトライは ListDevicesWithRetry を使う。
func ListDevices() ([]Device, error) {
	ctx, err := newContext()
	if err != nil {
		return nil, fmt.Errorf("audio: list devices: malgo init: %w", err)
	}
	// defer は LIFO: Uninit (内部状態解放) → Free (構造体解放) の順で実行。
	// malgo.AllocatedContext は Context を embed しているので Uninit() を直接呼べる。
	defer ctx.Free()
	defer ctx.Uninit()

	infos, err := ctx.Context.Devices(malgo.Capture)
	if err != nil {
		return nil, fmt.Errorf("audio: list devices: %w", err)
	}

	devices := make([]Device, 0, len(infos))
	for _, info := range infos {
		devices = append(devices, Device{
			ID:   info.ID.String(),
			Name: info.Name(),
		})
	}
	return devices, nil
}

// ListDevicesWithRetry は WASAPI 等の遅延初期化対策として
// exponential backoff (即時 → 1s → 2s → 4s) で ListDevices をリトライする。
//
// 戻り値:
//   - 成功 (デバイス ≥ 1): デバイス一覧, nil
//   - 失敗: nil, 最後の error
//
// Phase 1.13a P-5: 起動時 1 秒待機 + exponential backoff で最大 3 回 retry。
// 3 回失敗時は OS デフォルトにフォールバック + stderr ログ警告。
//
// 注: ブロッキング sleep を含むため、ゲームループ (Update) からは呼ばないこと。
// main.go でバックグラウンド goroutine から呼ぶ想定。
func ListDevicesWithRetry() ([]Device, error) {
	delays := []time.Duration{0, 1 * time.Second, 2 * time.Second, 4 * time.Second}
	var lastErr error
	for _, d := range delays {
		if d > 0 {
			time.Sleep(d)
		}
		devices, err := ListDevices()
		if err == nil && len(devices) > 0 {
			return devices, nil
		}
		lastErr = err
	}
	return nil, lastErr
}
