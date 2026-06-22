# GoTuber — Phase 2: Camera 入力 詳細設計

> **ステータス**: MediaPipe Face Landmarker 即採用で確定 (2026-06-17)、配信中可用性方針 (Section 1.1) 追加 (2026-06-22)
> **最終更新**: 2026-06-22 (Section 1.1 配信中可用性方針 + supervisor 3 層構成追加)
> **親プラン**: [PLAN.md](./PLAN.md) v0.4.6

**採用方針**: 顔 (頭) トラッキング + まばたきだけなら、YuNet (5 landmarks) ではなく **MediaPipe Face Landmarker** (Tasks API、478 landmarks) を即採用。1 つのモデルで頭の方向 (yaw/pitch/roll) と EAR (瞬き) を取れる。YuNet だと瞬き検出が別モデル or 別処理になり、依存が増えて結局 MediaPipe 構成と大差ないため。

**Phase 1 軽量性への影響**: MediaPipe は **Python サイドカー** 構成なので、GoTuber 本体 (Go バイナリ) のサイズ・依存は増えない。Python プロセス起動 + ZeroMQ 経由の IPC でつなぐ。

---

## 1. 目標

Webcam で **頭の方向 + 瞬き** を自動化し、配信者がマウスから解放される完全カメラ VTuber 体験の **最小版** を実現する。口パクは引き続き Phase 1.7 の malgo マイクが担当 (カメラと排他的にフォールバック)。

### 1.1 配信中可用性方針 (2026-06-22 確定)

カメラモードが有効な限り、**MediaPipe サイドカーは常時起動 + 自動再起動** する。配信中に tracker がクラッシュしても:

- **メインの GoTuber プロセスには一切影響しない** (panic recover 済み、goroutine 隔離済み)
- **tracker は supervisor loop で自動再起動** (exponential backoff、1s → 2s → 4s → 8s → 16s → 30s 上限、3 回成功でリセット)
- **1 秒以上顔未検出 / 1 秒以上 tracker dead → mouse follow にフォールバック** (見た目の自然さ優先)
- **tracker 再起動成功 → camera モードに自動復帰** (配信者が手動で再起動する必要ゼロ)

この方針により、長時間配信 (数時間〜) でも MediaPipe / OpenCV / ZeroMQ のいずれが transient に落ちても、人間の介入なしで継続可能。

実装は **3 層構成**:
- **L1 CameraTracker** (`internal/camera/capture.go`、Phase 2.2 完了、コードレビュー APPROVE): Go 側 panic 吸収 (defer recover + cleanupCtx パターン) + atomic 状態観測 (SentCount / IsRunning / LastErrorAt)
- **L2 MPClient** (`internal/camera/mpclient.go`、Phase 2.3 予定): ZeroMQ SUB port 5556 の channel close 検知
- **L3 supervisor loop** (`internal/camera/supervisor.go`、Phase 2.5 予定): CameraTracker / MPClient / mp_server.py サブプロセスを統合管理、`os/exec.CommandContext` で Python プロセス spawn・監視・再起動

---

## 2. ゴール / 非ゴール

### 2.1 ゴール (Phase 2 スコープ)

- 顔を左右上下に向けるとキャラが同じ方向を見る (yaw / pitch → 5×5 セル)
- 瞬きするとキャラが目をつむる (D → E/F 切替、Phase 1.6 のまばたき scheduler と優先度制御)
- マイク口パク (Phase 1.7) と同時動作 (B/A/C 切替は継続)
- カメラ消滅 / 顔未検出時は自動的にマウス追従 (Phase 1.5) にフォールバック
- 30 FPS 以上維持 (MediaPipe 推論は Python サイドカーの goroutine)
- **Phase 1 ビルドは引き続き軽量**: GoTuber.exe のサイズ・依存は変化なし
- **ヘッドレス展開**: Python プロセス起動できない環境ではマウスモードのみで起動 (graceful degradation)

### 2.2 非ゴール (Phase 2.5+ 検討枠)

- 口パク (口の縦横比検出) → Phase 1.7 の malgo が担当、カメラと排他
- 体の傾き (lean) 検出
- 腕 / 手の動き
- 全身 Live2D 風
- 3D モデル対応
- 複数カメラ対応

---

## 3. 設計の核: 5×5 で 2D 体左右をカバー

**顔を左右に向ける = 体を左右に向ける** (普通は一致) ので、5×5 (c0..c4) の yaw 軸がそのまま「**2D 体左右**」になる (Phase 2 着手時に PLAN.md で確定済み)。

| ユーザー動作 | 5×5 セル | キャラ表示 |
|---|---|---|
| 頭を左に向ける | c0 | 左向き |
| 頭を右に向ける | c4 | 右向き |
| 頭を上に向ける | r0 | 上目遣い |
| 頭を下に向ける | r4 | 下目遣い |

**新フォーマット不要、新モデル不要、新アセット不要**。Phase 2.4 のマッピングだけで対応。

カバーできないのは **body lean / 腕の動き / 足の動き** のみ。Phase 2.5+ で別軸追加 (必要時)。

---

## 4. アーキテクチャ: Python サイドカー + ZeroMQ IPC

### 4.1 なぜサイドカー構成か

MediaPipe の Go バインディング (mediapipe-go) は experimental で Windows CGo ビルドが地獄。Python 版は安定・枯れているので **Python プロセスを別起動して ZeroMQ でフレーム + 検出結果を受け渡し** する。

```
┌─────────────────────────────────────────────────────┐
│  GoTuber.exe (Go / Ebitengine)                      │
│  ┌──────────────┐   ZeroMQ PUB (port 5555)          │
│  │ CameraCapture│ ─── JPEG frame ───────────────┐    │
│  └──────────────┘                              │    │
│         ▲                                       ▼    │
│         │                              ┌──────────┐ │
│         │      ZeroMQ SUB (port 5556)  │ Detection│ │
│         └─────── landmarks JSON ───────│ results  │ │
│                                        └──────────┘ │
│  ┌────────────────────────────────────────────────┐  │
│  │ Avatar State mapper                            │  │
│  │ - yaw/pitch/roll → cell {r, c}                 │  │
│  │ - EAR < threshold → eyesClosed = true          │  │
│  │ - 1 sec 未検出 → fallback to mouse mode        │  │
│  └────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────┘
                              │
                              ▼
                ┌────────────────────────────┐
                │ mp_server.py (Python)      │
                │ - OpenCV webcam capture    │
                │ - MediaPipe Face Landmarker│
                │ - publishes JSON result    │
                │ - ~30 FPS on CPU            │
                └────────────────────────────┘
```

### 4.2 なぜ IPC に ZeroMQ か

- **TCP ソケットで十分**: ZeroMQ は低レベル過ぎ、Go の `net.Conn` + JSON over TCP で代替可能
- **しかし ZeroMQ を使う理由**:
  - 複数 subscriber への fan-out が容易 (将来ツールで GUI デバッグビューア作る時に便利)
  - PUB/SUB パターンで「フレーム送信 → 結果受信」が自然に書ける
  - Go binding (`github.com/pebbe/zmq4`) と Python binding (`pyzmq`) ともに安定
- **代替案**: Unix domain socket (Windows 非対応)、gRPC (over-engineering)、共有メモリ (プロセス分離の意味が薄い)

### 4.3 通信プロトコル

**Go → Python (frame publisher, port 5555)**:
```json
{
  "type": "frame",
  "seq": 12345,
  "width": 640,
  "height": 480,
  "data": "<base64-encoded JPEG>"
}
```

**Python → Go (detection result publisher, port 5556)**:
```json
{
  "type": "detection",
  "seq": 12345,
  "timestamp": 1718628000.123,
  "face_detected": true,
  "yaw": -12.5,
  "pitch": 3.2,
  "roll": 1.1,
  "ear_left": 0.28,
  "ear_right": 0.30,
  "face_center_x": 0.5,    // -1..1 normalized
  "face_center_y": 0.2     // -1..1 normalized
}
```

#### Wire Format (Phase 2.3d 確定)

| 方向 | wire 形式 | Phase |
|---|---|---|
| CameraTracker → mp_server.py (PUB 5555) | `<base64-JSON frame>` (prefix なし) | Phase 2.2 実装済 |
| mp_server.py → MPClient (PUB 5556) | `detection <detection JSON>` (prefix 付き) | Phase 2.3c 実装済 |

#### Topic Filter 一覧 (Phase 2.3d)

| 方向 | filter | Phase | 状態 |
|---|---|---|---|
| MPClient (SUB 5556) | `detection` | Phase 2.3b 先行実装 | ✅ コード反映済 |
| mp_server.py (SUB 5555) | `""` (全受信) | Phase 2.1 | ✅ 現状 |
| CameraTracker (PUB 5555) topic prefix 付与 | - | Phase 2.5+ | ⏸ 持ち越し |
| mp_server.py (PUB 5556) topic prefix 付与 | `"detection "` | Phase 2.3c | ✅ 確定 |

注: Phase 2.3c で mp_server.py 側に `DETECTION_TOPIC` 定数 + prefix 付き publish 実装済み、Phase 2.3b の mpclient.go と整合済み。Phase 2.5+ で frame topic prefix (Go 側 CameraTracker) 対応予定。

### 4.4 必要な Python 依存

**venv でカスタム環境を作成する** (Phase 2 で初めて Python 依存を導入するユーザー向けに推奨、YAGNI ではないが配布時の Python バージョン衝突を防ぐ):

```bash
# Phase 2 環境セットアップ (Phase 1 公開リリース後に追加される tools/setup-mp.sh / .ps1 で自動化)
python -m venv .venv-mp
source .venv-mp/bin/activate          # Linux / WSL / macOS
.venv-mp\Scripts\Activate.ps1         # Windows PowerShell

pip install -r tools/requirements-mp.txt
```

`tools/requirements-mp.txt`:
```
mediapipe>=0.10.14
opencv-python>=4.10.0
pyzmq>=26.0
numpy>=1.26.0
```

サイズ感:
- mediapipe: ~50MB (pip install)
- opencv-python: ~70MB
- pyzmq: ~5MB
- numpy: ~25MB (opencv-python と共有が多いが別途カウント)
- 合計 ~150MB の Python 環境 (Phase 2 ユーザー側セットアップ、venv 隔離で既存環境と非衝突)

**.gitignore 追加エントリ** (Phase 2 着手時に追加):
```gitignore
# Phase 2: MediaPipe Python サイドカー環境
.venv-mp/
__pycache__/
*.pyc
*.pyo
.pytest_cache/
```

venv を `phase-2-mp` のような専用名にして Phase 1 と区別することで、ユーザーが既に持っている Python 環境 (例: Stable Diffusion 用の venv) との衝突を防ぐ。`__pycache__/` と `*.pyc` / `*.pyo` は mp_server.py のバイトコードキャッシュで、コミット不要。

---

## 5. 実装項目

### Phase 2.0: 設計確定 (2026-06-17, ✅ 完了)

- MediaPipe Face Landmarker 即採用で確定 (YuNet 不採用)
- Python サイドカー構成で確定 (mediapipe-go 不採用)
- ZeroMQ IPC で確定 (TCP socket でも代替可だが ZeroMQ 採用)
- Phase 1 ビルドサイズへの影響ゼロ (Go 側依存なし)

### Phase 2.1: tools/mp_server.py 実装

- OpenCV `cv2.VideoCapture(0)` でカメラ起動
- MediaPipe Face Landmarker (Tasks API) で 478 ランドマーク抽出
- ランドマークから以下を計算:
  - yaw / pitch / roll: solvePnP (鼻先 1 + 額 10 + あご 152 で擬似 PnP)
  - EAR (Eye Aspect Ratio): 縦距離 = |上まぶた 159 - 下まぶた 145| / 横幅 = |外眼角 33 - 内眼角 133| (Soukupová & Čech 4-point canonical indices。コード参照: `tools/mp_server.py` の `_compute_ear`)
  - face_center: 鼻先 (1) の正規化座標 (-1..1)
- ZeroMQ PUB で port 5556 に JSON publish
- main loop は別 goroutine で asyncio ではなく threading
- FPS 計測 + ログ出力

### Phase 2.2: internal/camera/capture.go 実装

- `CameraCapture` struct: ZeroMQ SUB で port 5555 subscribe
- `CaptureLoop`: カメラは Go 側でも起動 (OpenCV 不要、`github.com/blackjack/webcam` で十分)、MJPEG → JPEG base64
- 別 goroutine で起動、channel に `frame` を publish
- **Phase 1 ビルドには影響なし**: `//go:build camera` でガード

### Phase 2.3: internal/camera/mpclient.go 実装

- `MPClient` struct: ZeroMQ SUB で port 5556 subscribe
- JSON parse → `DetectionResult` struct
- 最新結果のみ保持 (古いフレーム drop、channel buffer 1)

### Phase 2.4: internal/camera/mapper.go 実装

- `Mapper.Map(detection DetectionResult) (row, col, eyesClosed int)` 実装
- yaw (-90..90 deg) → col (0..4): `clampInt(int((yaw+90)/180*5), 0, 4)`
- pitch (-45..45 deg) → row (0..4): `clampInt(int((pitch+45)/90*5), 0, 4)` (tomari-guruguru と同じ r0=top 規約)
- roll は今回は無視 (Phase 2 スコープ外)
- EAR < 0.2 → eyesClosed = true (二値、ヒステリシス ±0.03)
- 顔未検出 → 最後に有効な値を 1 秒保持、その後 mouse mode fallback

### Phase 2.5: cmd/gotuber/main.go 統合

- 起動時チェックリスト:
  1. `--no-camera` フラグ → Phase 1 マウスモードで起動
  2. Python プロセス起動試行 (`tools/mp_server.py`)
  3. 起動失敗 → ログ warning + Phase 1 マウスモードで起動 (graceful degradation)
- Phase 1 と同じフロー、CameraCapture + MPClient を起動時に並行 goroutine 開始
- `internal/game/game.go` の mouse follow を `camera.Mapper` に切替 (Phase 1.5 の `mouse.Follower` はフォールバックとして残す)
- **優先度**: camera > audio mouth > mouse follow

**Supervisor loop (配信中可用性方針 Section 1.1 参照)** — 3 層構成:
- **L1: CameraTracker (Phase 2.2 Go 側)**: `defer recover` で Go panic 吸収、atomic 観測 (SentCount / IsRunning / LastErrorAt) で状態を supervisor に通知。**CameraTracker 単体は Go 側の panic (blackjack/webcam / zmq4) のみ対応**。Python 側エラーは検知不可。
- **L2: MPClient (Phase 2.3 Go 側)**: ZeroMQ SUB port 5556 の channel close 検知で mp_server.py の異常終了を検出 → supervisor に通知。
- **L3: supervisor loop (Phase 2.5 Go 側、internal/camera/supervisor.go)**: 上記 2 つの **Go 側コンポーネント** および **mp_server.py サブプロセス自体** (Python プロセス) を統合管理。`cmd/gotuber/main.go` が `os/exec.CommandContext` で mp_server.py を spawn、`Wait()` で exit code 監視、異常終了 → 再 spawn (exponential backoff)。
- exponential backoff: 1s → 2s → 4s → 8s → 16s → 30s 上限、3 回連続成功でリセット
- 5 回連続失敗 → Tweaks に "Camera Down — Manual Restart Required" 表示 + mouse follow 永続
- supervisor 自身の panic も defer recover でメインプロセス無影響
- `--no-camera` フラグまたは Tweaks 「Enable Camera = OFF」 → supervisor 起動停止 (mouse mode 永続)

**責務分担の明確化**:
- **Python プロセス (mp_server.py) のクラッシュ** → L3 supervisor が `os/exec` サブプロセス再 spawn + L2 MPClient のソケット再接続
- **Go 側 CameraTracker の panic (webcam/zmq4)** → L1 の defer recover で吸収 → L3 supervisor が `IsRunning() == false` 検知で `NewCameraTracker() → Start()` サイクル
- **MediaPipe / libzmq / OpenCV の Python 側クラッシュ** → mp_server.py 終了を L3 が spawn で再起動 (これら Python 側ライブラリは GoTuber プロセス外)
- **顔未検出 / tracker dead** → L1 + L2 が channel 経由で mapper に通知 → 1 秒タイムアウトで mouse follow フォールバック (L3 とは独立、mapper 層で処理)

### Phase 2.6: 瞬き EAR ヒステリシス

- 目が開いている時に EAR < 0.2 検出 → eyesClosed = true
- 目が閉じている時に EAR > 0.23 検出 → eyesClosed = false (ヒステリシス 0.03)
- ゆっくり瞬き (300ms 以上閉眼) はカメラ側で吸収、Phase 1.6 の scheduler は一旦無効化
- 自動瞬き (Phase 1.6) は **カメラ未使用時のみ** 有効化 (`camera_enabled == false` の場合のみ scheduler 動作)

### Phase 2.7: フェイルセーフ

- カメラ権限拒否 → 起動時 warning + Phase 1 マウスモードで起動
- デバイス未接続 → mp_server.py が 1 秒以内に detection 0 を publish → GoTuber 側 mapper が mouse follow fallback
- mp_server.py 起動失敗 → GoTuber は正常起動 (マウスモードのみ)、status に "camera unavailable"
- **mp_server.py クラッシュ (Python プロセス終了)** → supervisor loop (L3) が `os/exec` で再 spawn + MPClient (L2) ソケット再接続 (Section 1.1 配信中可用性方針)
- **mp_server.py 内部クラッシュ (MediaPipe / libzmq / OpenCV エラー)** → mp_server.py プロセスは通常 Python 例外で終了 → 上記 mp_server.py クラッシュと同じフロー
- **Go 側 CameraTracker の panic (blackjack/webcam / zmq4 / image encoding)** → CameraTracker (L1) の defer recover が panic を吸収 → supervisor loop (L3) が `IsRunning() == false` 検知で `NewCameraTracker() → Start()` サイクル
- 顔未検出 1 秒以上 → mouse follow mode に fallback (tracker dead 含む、Section 2.4 mapper)
- tracker 再起動 5 回連続失敗 → Tweaks に "Camera Down — Manual Restart Required" 表示 + mouse follow 永続 (配信者の判断で `--no-camera` 再起動可能)

### Phase 2.8: Tweaks パネル拡張

- 既存の Mic デバイス選択 UI の下に「Camera Device」ドロップダウン追加 (Phase 2.2 で実装した CameraCapture と連動)
- 「Enable Camera」チェックボックス追加 (デフォルト ON、OFF で Phase 1 マウスモード)
- 「Camera Mirror」チェックボックス追加 (左右反転、Phase 2.5+)
- すべて Phase 1.14.16 の Tweaks 永続化に乗せる (`[tweaks]` セクション追加)

### Phase 2.9: モデルファイル同梱

- MediaPipe Face Landmarker の `.task` ファイルをリポジトリに同梱 (`assets/models/face_landmarker.task`)
- Apache-2.0、Google 公式配布 (~3-5MB)
- ファイル URL: https://storage.googleapis.com/mediapipe-models/face_landmarker/face_landmarker/float16/latest/face_landmarker.task
- `.gitignore` に `assets/models/*.task` を入れない (Git LFS ではなく直接コミット、サイズ的に許容)

---

## 6. 完了基準 (DoD)

- [ ] Webcam 0 番デバイスで頭の動きがキャラに反映される (yaw/pitch → 5×5 セル)
- [ ] 顔を左右に向けるとキャラが振り向く (yaw → 列方向)
- [ ] 顔を上下に向けるとキャラが上目遣い/下目遣い (pitch → 行方向)
- [ ] 瞬きするとキャラが目をつむる (D → E/F 切替、EAR ベース)
- [ ] 顔を画面外に出すと 1 秒以内にマウス追従モードにフォールバック
- [ ] 30 FPS 以上維持 (Phase 1 の Ebitengine 描画ループ)
- [ ] **Python サイドカー起動失敗時も GoTuber.exe は正常起動** (graceful degradation)
- [ ] **Phase 1 ビルドサイズ・依存不変** (Go 側に `mediapipe` / `opencv-python` 依存なし)
- [ ] `--no-camera` フラグで Python 不要モード起動 (配信者セットアップ簡略化)
- [ ] カメラ切断時にクラッシュせずマウスモードへ移行
- [ ] Tweaks の `[tweaks]` セクションに `camera_enabled` / `mirror` 永続化
- [ ] **`go test ./...` 全パス、`go test -tags camera` でも全パス**
- [ ] **配信中可用性 (Section 1.1)**: CameraTracker を `kill -9 $mp_server_pid` で殺しても 5 秒以内に supervisor loop が自動再起動 → 頭追従復帰
- [ ] **クラッシュ耐性**: CameraTracker に panic を強制 (`kill -SIGSEGV` or Go 内部) してもメイン GoTuber は無影響、supervisor が 30 秒以内に再起動
- [ ] code-reviewer APPROVE
- [ ] yosia さん実機 visual test で頭追従 + 瞬き動作確認

---

## 7. 想定工数

2-3 週間 (Python サイドカー化で短縮)

内訳 (参考):
- mp_server.py (2.1): 0.5 週
- camera/capture.go (2.2): 0.5 週
- camera/mpclient.go + mapper.go (2.3, 2.4): 0.5 週
- 統合 + Tweaks 拡張 (2.5, 2.8): 0.5 週
- 仕上げ (2.6, 2.7): 0.5 週

---

## 8. 親プランとのクロスリファレンス

| 項目 | 親プラン参照 |
|---|---|
| Q8 (Phase 2 着手時に再評価、MediaPipe で確定) | [PLAN.md §11 Q8](./PLAN.md#11-未解決事項実装前確認) |
| R2 (OpenCV インストール摩擦) | [PLAN.md §7 R2](./PLAN.md#7-リスク--対策) (本 Phase では不採用に相当、Python サイドカー化で回避) |
| R6 (バイナリサイズ増) | [PLAN.md §7 R6](./PLAN.md#7-リスク--対策) (本 Phase では影響なし) |
| 4.2.1 パフォーマンス予算 (Phase 2 行) | [PLAN.md §4.2.1](./PLAN.md#421-パフォーマンス予算) |
| 8.1 メモリ予算 | [PLAN.md §8.1](./PLAN.md#81-メモリ予算) |
| 8.2 エラー UX (実行時致命的) | [PLAN.md §8.2](./PLAN.md#82-エラー-ux) |
| Phase 1 アーキテクチャの受け継ぎ | [PLAN.md §4](./PLAN.md#4-アーキテクチャ) |
| Phase 1.14 Tweaks 永続化スキーマ | [PHASE1.md §15](./PHASE1.md#15-phase-11416-tweaks-永続化-再起動後リセット問題--明示的-save-ボタン方式) |

---

## 9. 将来の検討 (Phase 2.5+)

Phase 2 完了後、必要に応じて独立フェーズで切り出す:

- **口パクのカメラ検出** (MAR): マイクと排他的に切替可能に
- **体の傾き (lean)**: 顔向きに依存しない独立軸。実装には body landmark 検出が必要
- **腕の動き**: MediaPipe Pose (33 landmarks) または MoveNet (17 keypoints) が必要
- **全身 Live2D 風**: アセットパイプライン再設計が必要、Phase 1〜3 との互換性ゼロ
- **複数カメラ**: 顔 + 全身の 2 カメラ運用
- **ハンドトラッキング**: 指の動き検出、特殊用途向け

これらは **Phase 2.5 / Phase 4+** として別フェーズで検討。今は Phase 2 完了優先。
