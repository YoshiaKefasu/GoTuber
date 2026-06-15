# GoTuber — Phase 3: VMC Protocol 出力 詳細設計

> **ステータス**: 未着手（Phase 2 完了後に着手）
> **最終更新**: 2026-06-15
> **親プラン**: [PLAN.md](./PLAN.md) v0.4.3

---

## 1. 目標

既存 VTuber ソフト（**VTube Studio** / **VSeeFace** / **EVMC4U** 等）に **モーション送信元（Performer）** として参加し、GoTuber のマイク/カメラ/マウス情報を **ブレンドシェイプ** として他ソフトのキャラに反映する。

「GoTuber をモーションキャプチャ元として使う」パターンを可能にする。

---

## 2. ゴール / 非ゴール

### 2.1 ゴール

- VMC Protocol（Marionette 側受信）として UDP 39539 に送信
- `/VMC/Ext/Blend/Val` で BlendShape 送信（A, I, U, E, O, Blink_L, Blink_R）
- `/VMC/Ext/Blend/Apply` 同期送信（30 Hz）
- VMC0 (VRM0) 形式で出力（VRM1 互換）
- 設定で ON/OFF、ホスト・ポート・送信周期変更可
- Phase 1 / Phase 2 のマイク・カメラ・まばたき情報を VMC に出力

### 2.2 非ゴール（Phase 4+ 検討枠）

- 全身ボーン送信
- 受信（Performer 側）実装
- WebSocket over OSC 拡張
- 認証 / 暗号化

---

## 3. VMC Protocol 概要

**Virtual Motion Capture Protocol (VMC Protocol)**: アバター motion 通信プロトコル。OSC over UDP/IP。

| 役割 | 送信 / 受信 | 例 |
|---|---|---|
| **Marionette** | 受信側（モーションを受けて描画など） | VTube Studio, VSeeFace, EVMC4U |
| **Performer** | 送信側（モーションを送る） | VirtualMotionCapture, MocapForAll |
| **Assistant** | 補助情報送信 | face2vmc, Sknuckle |

**GoTuber = Performer として動作**。Marionette に BlendShape を送る。

### 3.1 主要メッセージ

```
/VMC/Ext/Blend/Val (string){name} (float){value}
/VMC/Ext/Blend/Apply

/VMC/Ext/Set/Period (int){Status} (int){Root} (int){Bone} (int){BlendShape} (int){Camera} (int){Devices}
```

### 3.2 VRM0 / VRM1 互換

- VRM0 プリセット: `Joy, Angry, Sorrow, Fun, A, I, U, E, O, Blink_L, Blink_R`
- VRM1 プリセット: `happy, angry, sad, relaxed, aa, ih, ou, ee, oh, blinkLeft, blinkRight`
- **VRM0 形式で送信**（既存アプリとの互換性のため）

---

## 4. 実装項目

### Phase 3.0: スタック選定

go-osc のメンテ状況再評価:

- **案A: go-osc 利用**（R10 リスク承知）: `github.com/hypebeast/go-osc v0.0.0-20220308...`（2022-03 以降更新なし）
- **案B: 自前 OSC 送信実装**（推奨）: ~150 行、`net.UDPConn` で十分
  - 依存追加なし
  - バイナリサイズ +0
  - 単純な OSC 仕様のみ使う
  - go-osc の API も複雑

**推奨: 案B**。go-osc の 2022-03 以降更新なしリスクを取らず、送信部分だけなら自前で十分。

### Phase 3.1: UDP 39539 で送信

- `internal/vmc/client.go`
- `net.DialUDP("udp", nil, addr)` で送信用ソケット開く
- 設定の `host` / `port` に従って接続先決定
- 接続失敗時のリトライ戦略（指数バックオフ or 即座にエラー終了）

### Phase 3.2: /VMC/Ext/Blend/Val 送信

- 引数形式: `[Key1(string), Value1(float32), Key2(string), Value2(float32), ...]`
- 1 メッセージに複数の BlendShape をまとめて送る
- 設定の `blend_keys` マップで Key 変換
- mouth state (Closed/Half/Open) → `A` / `I` / `U` / `E` / `O` の値
  - 例: MouthClosed=0.0, MouthHalf=0.5, MouthOpen=1.0
- 口の lip-sync は mouth 状態を 5 母音に分配するロジックも検討

### Phase 3.3: /VMC/Ext/Blend/Apply 送信

- 1 フレームの最後に Apply を送信（受信側でバッファフラッシュ）
- 30 Hz 送信（VSync と非同期で OK）

### Phase 3.4: VMC0 形式

- VRM0 プリセット（A, I, U, E, O, Blink_L, Blink_R）を使用
- VRM1 受信側との互換性のため、VRM0 形式で送信
- 受信側アプリが VRM0 → VRM1 変換を担当

### Phase 3.5: 設定

`config/default.yaml`:
```yaml
vmc:
  enabled: false
  host: "127.0.0.1"
  port: 39539
  send_rate_hz: 30
  blend_keys:
    blink_left: "Blink_L"
    blink_right: "Blink_R"
    a: "A"
    i: "I"
    u: "U"
    e: "E"
    o: "O"
```

### Phase 3.6: Phase 1 / 2 との統合

- Phase 1 の `audio.MouthState` → VMC の mouth blend
- Phase 1 の `blink.Scheduler.State` → VMC の blink blend
- Phase 2 の `camera.MouthAR` / `camera.EyeEAR` → VMC の対応 blend（カメラが優先）
- 優先順位: カメラ > 音声 > デフォルト

---

## 5. 完了基準 (DoD)

- [ ] VTube Studio 受信テストでブレンドシェイプ反映
- [ ] VSeeFace 0.91+ 受信テストで反映
- [ ] バイナリ +0.5 MB 以下（自前実装時）
- [ ] `go test ./... -v -race` 全パス
- [ ] 設定で ON/OFF 可能
- [ ] 送信周期 30 Hz 維持
- [ ] OSC バイトオーダー（big-endian）が正しい（VMC 仕様準拠）

---

## 6. 想定工数

1〜2 週間

内訳:
- Phase 3.0（自前 OSC 送信実装）: 2〜3 日
- Phase 3.1〜3.4（VMC メッセージ送信）: 2〜3 日
- Phase 3.5（設定 + Phase 1/2 統合）: 1〜2 日
- Phase 3.6（テスト + VTube Studio 検証）: 2 日

---

## 7. 親プランとのクロスリファレンス

| 項目 | 親プラン参照 |
|---|---|
| §2.1 スタック選定（go-osc R10 リスク） | [PLAN.md §2.1](./PLAN.md#2-スタック選定) |
| §2.2 却下した選択肢 | [PLAN.md §2.2](./PLAN.md#2-スタック選定) |
| §4.4 設定ファイル（vmc セクション） | [PLAN.md §4.4](./PLAN.md#4-アーキテクチャ) |
| §7 R7（VMC Protocol 仕様の網羅性） | [PLAN.md §7](./PLAN.md#7-リスク--対策) |
| §7 R10（go-osc メンテナンス停止） | [PLAN.md §7](./PLAN.md#7-リスク--対策) |
| §11 Q11（署名・公証） | [PLAN.md §11](./PLAN.md#11-未解決事項実装前確認) |

---

## 8. 関連ドキュメント

- VMC Protocol 仕様（公式）: https://protocol.vmc.info/english.html
- VMC Protocol 仕様（Marionette 受信）: https://protocol.vmc.info/marionette-spec.html
- VTube Studio: https://denchisoft.com/
- VSeeFace: https://www.vseeface.icu/
- EVMC4U (Unity で VRM 受信): https://github.com/ashinnotfound/EVMC4U
- go-osc (案A、非推奨): https://github.com/hypebeast/go-osc
