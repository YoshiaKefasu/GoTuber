# GoTuber — Phase 2: Camera 入力 詳細設計

> **ステータス**: MediaPipe Face Landmarker 即採用で確定 (2026-06-17)、配信中可用性方針 (Section 1.1) 追加 (2026-06-22)
> **最終更新**: 2026-06-23 (Phase 2.10 Windows native camera 未完了を明記)
> **親プラン**: [PLAN.md](./PLAN.md) v0.4.6

**採用方針**: 顔 (頭) トラッキング + まばたきだけなら、YuNet (5 landmarks) ではなく **MediaPipe Face Landmarker** (Tasks API、478 landmarks) を即採用。1 つのモデルで頭の方向 (yaw/pitch/roll) と EAR (瞬き) を取れる。YuNet だと瞬き検出が別モデル or 別処理になり、依存が増えて結局 MediaPipe 構成と大差ないため。

**Phase 1 軽量性への影響**: MediaPipe は **Python サイドカー** 構成なので、GoTuber 本体 (Go バイナリ) のサイズ・依存は増えない。Python プロセス起動 + **localhost TCP JSONL** の IPC でつなぐ。

---

## 1. 目標

Webcam で **頭の方向 + 瞬き** を自動化し、配信者がマウスから解放される完全カメラ VTuber 体験の **最小版** を実現する。口パクは引き続き Phase 1.7 の malgo マイクが担当 (カメラと排他的にフォールバック)。

### 1.1 配信中可用性方針 (2026-06-22 確定)

カメラモードが有効な限り、**MediaPipe サイドカーは常時起動 + 自動再起動** する。配信中に tracker がクラッシュしても:

- **メインの GoTuber プロセスには一切影響しない** (panic recover 済み、goroutine 隔離済み)
- **tracker は supervisor loop で自動再起動** (exponential backoff、1s → 2s → 4s → 8s → 16s → 30s 上限、3 回成功でリセット)
- **1 秒以上顔未検出 / 1 秒以上 tracker dead → mouse follow にフォールバック** (見た目の自然さ優先)
- **tracker 再起動成功 → camera モードに自動復帰** (配信者が手動で再起動する必要ゼロ)

この方針により、長時間配信 (数時間〜) でも MediaPipe / OpenCV / TCP IPC のいずれが transient に落ちても、人間の介入なしで継続可能。

実装は **3 層構成**:
- **L2 MPClient** (`internal/camera/mpclient.go`): localhost TCP listener (`127.0.0.1:5556`) で detection JSONL 受信
- **L3 supervisor loop** (`internal/camera/supervisor.go`): MPClient / mp_server.py サブプロセスを統合管理、`os/exec.CommandContext` で Python プロセス spawn・監視・再起動

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

## 4. アーキテクチャ: Python サイドカー + localhost TCP JSONL IPC

### 4.1 なぜサイドカー構成か

MediaPipe の Go バインディング (mediapipe-go) は experimental で Windows CGo ビルドが地獄。Python 版は安定・枯れているので **Python プロセスを別起動して localhost TCP で検出結果を受け渡し** する。

```
┌─────────────────────────────────────────────────────┐
│  GoTuber.exe (Go / Ebitengine)                      │
│  ┌────────────────────────────────────────────────┐  │
│  │ MPClient (TCP listener 127.0.0.1:5556)        │  │
│  │ - newline-delimited detection JSON            │  │
│  └────────────────────────────────────────────────┘  │
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
│ - sends JSONL result       │
                │ - ~30 FPS on CPU            │
                └────────────────────────────┘
```

### 4.2 なぜ IPC に localhost TCP を使うか

- **標準ライブラリだけで完結**: Go `net` / Python `socket` で十分
- **Windows の native 依存が消える**: `zmq.h` / libzmq / pyzmq が不要
- **今回の要件に十分**: 1 プロセスの Python sidecar から 1 プロセスの GoTuber へ detection JSON を流すだけ
- **代替案**: ZeroMQ (過剰 + native 依存)、Unix domain socket (Windows 非互換)、gRPC (over-engineering)

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
numpy>=1.26.0
```

MediaPipe Face Landmarker モデルは Phase 2.9 で `assets/models/face_landmarker.task`
に同梱済み。通常起動はネットワーク不要で、`mp_server.py` の fallback auto-download は
同梱モデルが欠けた場合のみ使う。

サイズ感:
- mediapipe: ~50MB (pip install)
- opencv-python: ~70MB
- numpy: ~25MB (opencv-python と共有が多いが別途カウント)
- 合計 ~145MB の Python 環境 (Phase 2 ユーザー側セットアップ、venv 隔離で既存環境と非衝突)

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
- localhost TCP JSONL IPC で確定 (Phase 2.10 で ZeroMQ 廃止)
- Phase 1 ビルドサイズへの影響ゼロ (Go 側依存なし)

### Phase 2.1: tools/mp_server.py 実装

- OpenCV `cv2.VideoCapture(0)` でカメラ起動
- MediaPipe Face Landmarker (Tasks API) で 478 ランドマーク抽出
- ランドマークから以下を計算:
  - yaw / pitch / roll: solvePnP (鼻先 1 + 額 10 + あご 152 で擬似 PnP)
  - EAR (Eye Aspect Ratio): 縦距離 = |上まぶた 159 - 下まぶた 145| / 横幅 = |外眼角 33 - 内眼角 133| (Soukupová & Čech 4-point canonical indices。コード参照: `tools/mp_server.py` の `_compute_ear`)
  - face_center: 鼻先 (1) の正規化座標 (-1..1)
- localhost TCP (`127.0.0.1:5556`) に newline-delimited JSON publish
- main loop は別 goroutine で asyncio ではなく threading
- FPS 計測 + ログ出力

### Phase 2.2: internal/camera/capture.go 実装

- 旧設計では Go 側 frame publisher を担当していた
- **Phase 2.10 で廃止**: webcam capture は `mp_server.py` に一本化
- 現在は `capture.go` / `capture_test.go` ともに `//go:build ignore`

### Phase 2.3: internal/camera/mpclient.go 実装

- `MPClient` struct: localhost TCP listener (`127.0.0.1:5556`) で detection JSONL を受信
- JSON parse → `DetectionResult` struct
- 最新結果のみ保持 (古いフレーム drop、channel buffer 1)

### Phase 2.4: internal/camera/mapper.go 実装

- 公開 API (free function 5 個 + Cell/BlinkState 型):
  - `YawToCol(yawDeg float64) int`: yaw (-30..+30 deg) → col (0..4)、`math.Round` 四捨五入、clamp
  - `PitchToRow(pitchDeg float64) int`: pitch (-20..+20 deg) → row (0..4)、Y軸反転 (pitch +20 = 上 = row 0)、clamp
  - `EARToBlink(earLeft, earRight float64) BlinkState`: 両耳平均で Open/Half/Closed 3 状態
  - `FaceCenterToNormalized(faceX, faceY float64, w, h int) (nx, ny float64, ok bool)`: pixel → [-1,1]、mouse.Follower.Update と同一式
  - `FaceDetected(lastDetected, now float64) bool`: 1 秒タイムアウト (`now - lastDetected < 1.0`、境界ジャストで false)
- 値型: `Cell{Col, Row int}` (5x5 atlas セル座標)、`BlinkState int` (BlinkOpen=0/BlinkHalf=1/BlinkClosed=2)
- 閾値定数 (private): `yawMinDeg=-30`, `yawMaxDeg=+30`, `pitchMinDeg=-20`, `pitchMaxDeg=+20`, `earOpenThreshold=0.22`, `earClosedThreshold=0.10`, `faceDetectionTimeoutSec=1.0`
- 設計判断:
  - 範囲を ±90°/±45° から ±30°/±20° に縮小 (画面前で使う現実的な head pose 範囲、profile view で fallback を促す)
  - EAR は二値ではなく 3 状態 (Half で D→E→F 滑らかな遷移、ヒステリシスは Phase 2.6 で別レイヤ被せ)
  - `int(math.Round)` で mouse.Follower.Cell() の四捨五入規約と一致 (境界 twich 防止)
  - EAR 範囲外 (< 0 / > 0.5) は Open にフェイルセーフ (MediaPipe noise で「閉じ誤判定」を避ける)
  - build tag なし: 純粋関数のみ、Phase 1 ビルドに含まれる (Phase 1 テストにも露出)
- 顔未検出 → 1 秒以内は最後のセル保持、その後 mouse mode fallback (Section 1.1 配信中可用性方針)
- roll は今回は無視 (Phase 2 スコープ外)
- EAR ヒステリシスは Phase 2.6 で別レイヤ実装、ここでは単純な閾値のみ

### Phase 2.5: cmd/gotuber/main.go 統合

- 起動時チェックリスト:
  1. `--no-camera` フラグ → Phase 1 マウスモードで起動
  2. Python プロセス起動試行 (`tools/mp_server.py`)
  3. 起動失敗 → ログ warning + Phase 1 マウスモードで起動 (graceful degradation)

### Phase 2.5 環境要件

- Python sidecar 実行時は `mediapipe`, `opencv-python`, `numpy` が必要
- gcc PATH 必須 (Windows: `C:\gcc\mingw64\bin` を PATH 追加、Phase 1.13a で設定済)
- `-tags camera` ビルド自体は libzmq 不要になった
- テスト実行: `go build -tags camera ./... && go test -tags camera ./...`

- Phase 1 と同じフロー、CameraCapture + MPClient を起動時に並行 goroutine 開始
- `internal/game/game.go` の mouse follow を `camera.Mapper` に切替 (Phase 1.5 の `mouse.Follower` はフォールバックとして残す)
- **優先度**: camera > audio mouth > mouse follow

**Supervisor loop (配信中可用性方針 Section 1.1 参照)** — 3 層構成:
- **L2: MPClient (Phase 2.3 Go 側)**: localhost TCP listener で detection JSONL を受信。接続断は read error / EOF として検知 → supervisor に通知。
- **L3: supervisor loop (Phase 2.5 Go 側、internal/camera/supervisor.go)**: MPClient と **mp_server.py サブプロセス自体** (Python プロセス) を統合管理。`cmd/gotuber/main.go` が `os/exec.CommandContext` で mp_server.py を spawn、`Wait()` で exit code 監視、異常終了 → 再 spawn (exponential backoff)。
- exponential backoff: 1s → 2s → 4s → 8s → 16s → 30s 上限、3 回連続成功でリセット
- 5 回連続失敗 → Tweaks に "Camera Down — Manual Restart Required" 表示 + mouse follow 永続
- supervisor 自身の panic も defer recover でメインプロセス無影響
- `--no-camera` フラグまたは Tweaks 「Enable Camera = OFF」 → supervisor 起動停止 (mouse mode 永続)

**責務分担の明確化**:
- **Python プロセス (mp_server.py) のクラッシュ** → L3 supervisor が `os/exec` サブプロセス再 spawn + L2 MPClient の再接続待機
- **Go 側 MPClient の panic / 接続断** → L2 が read error / panic recover → L3 supervisor が ReceiveLoop 再起動
- **MediaPipe / OpenCV の Python 側クラッシュ** → mp_server.py 終了を L3 が spawn で再起動 (これら Python 側ライブラリは GoTuber プロセス外)
- **顔未検出** → L2 の最新 detection から mapper に通知 → 1 秒タイムアウトで mouse follow フォールバック

### Phase 2.6: 瞬き EAR ヒステリシス

- Phase 2.4 の `EARToBlink` は 3 状態 (Open/Half/Closed) を返す単純な 2 段しきい値
- Phase 2.6 でヒステリシスを別レイヤ型 (`BlinkFilter`) で被せる (mapper 単体は変更しない、camera パッケージ内に独立した stateful 型として実装)
- Phase 2.7 で supervisor.go に統合予定 (tickCell 内で BlinkFilter.Update() を呼び、EyesClosed() 戻り値に渡す)
- ヒステリシス方針 (3 状態 + ヒステリシス):
  - Open → Half: earAvg < 0.20 (下降しきい値、Phase 2.4 の Open 0.22 から僅かに下げてヒステリシス確保)
  - Half → Open: earAvg > 0.24 (上昇しきい値、Phase 2.4 の Open 0.22 から僅かに上げてヒステリシス確保)
  - Half → Closed: earAvg < 0.10 (Phase 2.4 の Closed と同じ)
  - Closed → Half: earAvg > 0.14 (Closed から 0.04 上げる)
- ゆっくり瞬き (300ms 以上閉眼) はカメラ側で吸収、Phase 1.6 の scheduler は一旦無効化
- 自動瞬き (Phase 1.6) は **カメラ未使用時のみ** 有効化 (`camera_enabled == false` の場合のみ scheduler 動作)

### Phase 2.7: フェイルセーフ + BlinkFilter 統合 (実装完了予定)

- カメラ権限拒否 → 起動時 warning + Phase 1 マウスモードで起動
- デバイス未接続 → mp_server.py が 1 秒以内に detection 0 を publish → GoTuber 側 mapper が mouse follow fallback
- mp_server.py 起動失敗 → GoTuber は正常起動 (マウスモードのみ)、status に "camera unavailable"
- supervisor.go に BlinkFilter 統合 (Phase 2.6 から)
  - tickCell 内で EARToBlink → BlinkFilter.Update() 置換
  - EyesClosed() は BlinkFilter.State() == BlinkClosed
  - switchToMouseLocked() で blinkFilter.Reset() (grace period 中の state 引き継ぎ防止)
- supervisor.go に mp_server.py サブプロセス管理追加
  - startMPServer() / stopMPServer() / monitorMPServer()
  - exponential backoff 1→30秒 (5回失敗で manual restart 要求)
  - cross-platform: Windows (python.exe) + Unix (python3)
- supervisorLoop 内で monitorMPServer() を毎フレーム呼び出し
- camera_hook_camera.go で supervisor.startMPServer() を call
- 配信中可用性方針 (Section 1.1) との整合:
  - mp_server.py クラッシュ → 1秒後に自動再起動、5回失敗で停止 (mouse fallback で継続)
  - プロセスは panic recover + graceful shutdown 必須
  - webcam デバイス解放は Stop() で確実実行
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

### Phase 2.9: モデルファイル同梱 (2026-06-23, ✅ 完了)

- MediaPipe Face Landmarker の `.task` ファイルをリポジトリに同梱済み (`assets/models/face_landmarker.task`)
- Apache-2.0、Google 公式配布 (3,758,596 bytes、約 3.6 MB)
- ファイル URL: https://storage.googleapis.com/mediapipe-models/face_landmarker/face_landmarker/float16/latest/face_landmarker.task
- `.gitignore` に `assets/models/*.task` を入れない (Git LFS ではなく直接コミット、サイズ的に許容)
- `tools/mp_server.py --model-path` のデフォルトは `assets/models/face_landmarker.task`
- 同梱モデルが存在する通常ケースでは auto-download をスキップする。fallback download は
  モデル欠落時のみ維持し、`--no-auto-download` 指定時は即エラー終了する。
- ライセンス帰属: MediaPipe Face Landmarker task model by Google / MediaPipe、Apache-2.0。

### Phase 2.10: Windows Native Camera 対応 (2026-06-23, ✅ 実装完了)

> **現状**: Phase 2.10 で Windows native build blocker を除去済み。
> `build.ps1 -Camera` は **`bin/gotuber-camera.exe`** を生成できる。
> Go 側の camera 通信は **localhost TCP JSONL** に切り替わり、
> `blackjack/webcam` / `zmq.h` / `pyzmq` 依存は camera build 経路から外れた。

#### 解消した根本原因 (3 要因)

旧 `-tags camera` ビルドが Windows で失敗していた原因は次の 3 つだった:

| 要因 | ファイル | 依存 | 問題 |
|---|---|---|---|
| **1. webcam capture** | `internal/camera/capture.go` | `github.com/blackjack/webcam` | Linux V4L2 前提 (`/dev/videoN`, `ioctl`, `unix.Syscall`)。Windows には `/dev/videoN` がなく、`unix.Syscall` は undefined |
| **2. ZeroMQ transport** | `internal/camera/mpclient.go` | `github.com/pebbe/zmq4` | `zmq.h` (libzmq dev header) が必要。Windows 側セットアップが前提だった |
| **3. type 依存** | `internal/camera/supervisor.go` | `*CameraTracker` (capture.go の型) | `NewSupervisor(tracker *CameraTracker, ...)` が capture.go の型に直接依存していた |

Phase 2.10 ではこれを次のように解消した:
- `capture.go` / `capture_test.go` は `//go:build ignore` に変更して現役経路から除外
- `mpclient.go` は `zmq4` を廃止し、`127.0.0.1:5556` の TCP listener + JSONL 受信へ置換
- `supervisor.go` から `CameraTracker` 直依存を除去
- `mp_server.py` は `pyzmq` を廃止し、TCP client として GoTuber へ detection JSON を送る

#### 採用した実装案

**採用: 案 B をさらに進めて transport も TCP 化**

理由:
1. `mp_server.py` は既に `cv2.VideoCapture(0)` でカメラ起動しているため、Go 側 capture は冗長
2. `zmq4` / `pyzmq` / `zmq.h` をまとめて落とせる
3. Go `net` / Python `socket` の stdlib だけで十分
4. Windows native build の blocker を build 時点で解消できる

#### 実装結果

| 項目 | ファイル | 結果 |
|---|---|---|
| webcam 依存除去 | `internal/camera/capture.go`, `capture_test.go` | `//go:build ignore` 化して現役経路から除外 |
| transport 置換 | `internal/camera/mpclient.go`, `tools/mp_server.py` | ZeroMQ → localhost TCP JSONL |
| supervisor 整理 | `internal/camera/supervisor.go`, `cmd/gotuber/camera_hook_camera.go` | `CameraTracker` 直依存除去 |
| build script | `scripts/build.ps1`, `scripts/dev.ps1`, `scripts/build.sh` | Windows native `gotuber-camera.exe` 生成経路へ更新 |
| Python 依存 | `tools/requirements-mp.txt` | `pyzmq` 削除 |

#### 完了条件

- [x] `capture.go` の `blackjack/webcam` 依存を排除
- [x] `supervisor.go` の `CameraTracker` 直依存を剥がす
- [x] `mpclient.go` を ZeroMQ から localhost TCP JSONL へ置換
- [x] `build.ps1 -Camera` が `bin/gotuber-camera.exe` を生成
- [x] `go test ./...` / `go vet ./...` / `go build -tags camera` が通る
- [ ] Windows 実機で `bin/gotuber-camera.exe` を起動し、Tweaks panel に Camera セクションが表示
- [ ] Windows 実機で `mp_server.py` が webcam capture + MediaPipe 推論を実行

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
- [ ] **Windows native camera build**: `build.ps1 -Camera` で `bin/gotuber-camera.exe` が生成され、Windows 上で Tweaks panel に Camera セクションが表示される (Phase 2.10)
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
