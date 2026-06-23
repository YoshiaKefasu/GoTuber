# GoTuber — 詳細プラン

> **ステータス**: v0.4.6（Q1, Q3, Q4, Q12 確定、Q6 post_release 化、Q8 MediaPipe 即採用で確定、配信中可用性方針追加 (Section 1.1)）
> **作成日**: 2026-06-15 / 改訂 2026-06-15（v0.4: レビュー反映、v0.4.3: Q 確定反映）/ 2026-06-16（v0.4.4: Q6 post_release 化）/ 2026-06-17（v0.4.5: Q8 MediaPipe 即採用で確定）
> **ベース**: `tomari-guruguru`（React/Vite/JSX）→ **Golang 完全書き換え**（確定）
> **ターゲット OS**: Windows / Linux / macOS
> **ビルド環境**: Windows 10/11（PowerShell + MSVC）または WSL Ubuntu（gcc）。KASOU は runtime 専用
> **ランタイム検証環境**: KASOU（Debian 13 Xfce, x86_64）

---

## 0. 環境前提

### 0.1 ビルド環境（開発機 = この Windows PC）

ビルドは以下の **どちらか一方** で実施する。KASOU（MiniPC）は CPU が非力なため **ビルドには使わない**。

| 選択肢 | 用途 | CGo コンパイラ |
|---|---|---|
| **Windows native (PowerShell + MSVC)** | ローカルで Windows バイナリを直接実行・確認 | MSVC（Visual Studio Build Tools 2022） |
| **WSL Ubuntu** | Linux バイナリをビルドして KASOU にデプロイ、またはクロスビルド | gcc + libasound2-dev 等 |

推奨は **WSL Ubuntu 主軸**：KASOU と同じ Debian 系のため動作互換性が高い。

### 0.2 ランタイム環境（KASOU）

- WSL Ubuntu（または Windows）でビルドした Linux バイナリを `scp` で KASOU へ転送
- KASOU で `./gotuber` を直接実行
- マイク・カメラ・X11 が必要

### 0.3 必要ツール

- **Go 1.25+**（Ebitengine v2.9 + `golang.org/x/image v0.42.0` 要件）
- C コンパイラ（MSVC または gcc）
- Phase 2: Python 3.9+ + mediapipe 0.10.14+ + opencv-python + numpy（`tools/requirements-mp.txt` 参照、Go 側ビルドタグ `camera` のみ）
- git, ffmpeg, ffprobe（スライス生成時）
- Python 3 + `pip install -r tools/requirements.txt`（スライス生成時のみ）

---

## 0.5 設計判断ログ

### 決定 (2026-06-15): 案A（pure Go 書き直し、PLAN.md v0.3 通り）

3 アーキテクチャ候補（pure Go 書き直し / Wails v2 ハイブリッド / webview/webview_go ラッパー）を比較検討の上、**案A（pure Go 書き直し）** を採用確定。

「シンプル + 軽量」を **ランタイム RAM の少なさ + 単一プロセス + WebView 依存ゼロ** と定義。24/7 連続運用する Windows 優先 VTuber ソフトではランタイム効率が最重要。

#### 比較サマリ

| 軸 | 案A pure Go | 案B Wails | 案B' webview |
|---|---|---|---|
| ランタイム RAM | **8〜15 MB** | 40〜80 MB | 35〜70 MB |
| バイナリサイズ | 8〜15 MB | 5〜10 MB | 5〜10 MB |
| WebView 依存 | **なし** | WebView2 / WKWebView / WebKitGTK | 同左 |
| 単一プロセス | **はい** | いいえ（IPC） | 同左 |
| 想定工期 | **1〜2 週** | 1 週 | 1 週 |
| コード行数 | ~560 Go | 100 Go + 1k React | 100 Go + 1k React |

却下理由: カメラ入力するなら gocv で CGo 必須なので「WebView で CGo 回避」の旨味なし、24/7 で WebView メモリ・CPU オーバーヘッドが定常的に発生、OBS 透明キャプチャの実装品質は Ebitengine が優位、既存 React コードの実ロジックは 575 行のみ（残りは React 糊）。

### 追加決定 (v0.4): オーディオアーキテクチャ = **malgo 完結**

Ebitengine audio は PCM デコーダーとしてのみ利用、Player/Context は起動しない。

**理由**:
1. **malgo 1 個**で「マイク入力 + ファイル再生 + スピーカー出力 + RMS 解析」を統一可能
2. Ebitengine の `audio/{mp3,wav,vorbis}` パッケージは PCM Reader として使える（Player 不要）
3. CGo 依存を malgo 1 個に集約でき、Section 2.3 の「CGo 最小化」と整合

**却下した代替案**: Ebitengine audio 完結 → マイク入力取得に別 CGo ライブラリ（portaudio 等）が追加で必要、結局 malgo と同じかそれ以上の CGo になる。

---

## 1. ゴール & スコープ

### 1.1 ゴール

- `tomari-guruguru` の **Golang 完全書き換え**（pure Go、ハイブリッド不可）
- 画像ベース PNGTuber の **5×5 角度 × 6 状態 = 150 フレーム** を維持
- 起動時間・メモリを **1/3 〜 1/10** へ（実測後に確定）
- 単一実行ファイル配布（Phase 1: 19 MB、Phase 2: Go バイナリ変化なし (19 MB) + Python env +125 MB）
- **NEW**: Webcam 入力で頭追従・まばたきを自動化（口パクは Phase 1.7 の malgo マイクが継続担当）
- **NEW（任意）**: VMC Protocol 出力で VTube Studio / VSeeFace 連携

### 1.2 スコープ

| 区分 | 内容 |
|---|---|
| **Must (Phase 1)** | 5×5×6 描画 / マウス追従 / **メインはメインマイクで同時 Realtime 口パク** / 自動まばたき / 透過ウィンドウ+クリックスルー / Tweaks パネル / WebP+PNG 対応 |
| **Should (Phase 2+)** | Webcam 頭の方向 + 瞬きトラッキング（Phase 2、MediaPipe 即採用で確定）/ VMC Protocol 出力（Phase 3） |
| **Deferred (Phase 1.5+)** | 音声ファイル口パク（Q6 で保留、ユーザー判断：「メインマイクで同時 Realtime 口パク」が主目的）。**2026-06-17 更新**: Phase 1 公開リリース後の post-release 扱いに変更。詳細 → `docs/post_release.md` Section 1.5 |
| **Won't** | Live2D・3D モデル対応 / 仮想カメラ（OBS ウィンドウキャプチャで代替）/ アセット同梱（権利上不可） |

---

## 2. スタック選定

### 2.1 主要ライブラリ（バージョン固定）

| 役割 | 採用 | バージョン | 理由 |
|---|---|---|---|
| 描画・ループ・入力 | `github.com/hajimehoshi/ebiten/v2` | **v2.9.9** (2026-03-03) | pure Go、`ScreenTransparent` 透過、`SetWindowMousePassthrough` 対応（**`Update()` 初回内で呼ぶ必要**、Issue #3222）。**Go 1.25+ 必須** |
| マイク・ファイル再生・スピーカー（**malgo 完結**） | `github.com/gen2brain/malgo` | v0.11.25 (2026-05-13) | miniaudio バインディング、CGo 1 個で「入力 + 出力 + ファイル」を統一 |
| 音声ファイル PCM デコード | `github.com/hajimehoshi/ebiten/v2/audio/{mp3,wav,vorbis}` | Ebitengine 同梱 | **Reader としてのみ利用**、Player/Context は起動しない |
| 画像デコード（WebP） | `golang.org/x/image/webp` | v0.42.0 (2026-06-08) | **Go 1.25 必須**。WebP は標準 `image` 非対応のため必須。`image.RegisterFormat("webp", ...)` で登録 |
| カメラ入力（Phase 2） | `mediapipe + opencv-python + numpy` (Python) + Go stdlib `net` | mediapipe 0.10.14+, opencv-python 4.10+, numpy 1.26+ | **2026-06-17 採用 / 2026-06-23 改訂**: MediaPipe Face Landmarker Tasks API を Python サイドカー構成で採用。Phase 2.10 で ZeroMQ を廃止し localhost TCP JSONL に一本化 |
| UI ウィジェット | `github.com/ebitenui/ebitenui` | v0.7.3 (2026-03-16) | Ebitengine 用 retained-mode UI |
| OSC（Phase 3） | `github.com/hypebeast/go-osc` | **v0.0.0-20220308234300-cec5a8a1e5f5（2022-03 以降更新なし — R10 参照）** | Phase 3 は自前最小実装（~150 行）を併用 |
| 設定 | `gopkg.in/yaml.v3` | v3 | キャラクター設定 |
| ログ | `log/slog` (標準) | — | stderr / `~/.local/share/gotuber/gotuber.log` / `GOTUBER_LOG_FILE` 環境変数で切替。10 MB × 3 世代ローテーション |
| CJK フォント | `assets/fonts/NotoSansCJKjp-Regular.otf` | 同梱 | 埋め込み。OFL ライセンス、`go:embed` |

### 2.2 却下した選択肢

| 案 | 却下理由 |
|---|---|
| **Wails v2** | 「Golang 書き換え」要件に合わない。クリック透過も Wails Issue #2969 で進行中 |
| **Fyne** | 透過ウィンドウ安定性、カスタム描画自由度 |
| **Gio** | 学習コスト、UI ライブラリ弱い |
| **Ebitengine v3 alpha** | v3.0.0-alpha.96 で本番利用は時期尚早。クリック透過系の修正進行中 |
| **`github.com/chai2010/webp`** | CGo + libwebp 依存。pure Go で行きたい |
| **`github.com/scgolang/osc`** | go-osc 代替候補だが実績少。Phase 3 開始時に再評価 |
| **Ebitengine audio 完結（オーディオ）** | マイク入力に別 CGo ライブラリ必要、結局 malgo と同じ負担 |

### 2.3 CGo 依存とビルドタグ

| Phase | CGo 依存 | ビルドタグ | クロスコンパイル |
|---|---|---|---|
| Phase 1 | **malgo のみ**（マイク + ファイル + スピーカー + RMS を統一） | （タグなし） | WSL → Windows: `GOOS=windows CGO_ENABLED=1` で可能（OpenCV 無しなので） |
| Phase 2 | + mediapipe/opencv-python/numpy (Python サイドカー) | `-tags camera` | **Windows native 対応済み**: Phase 2.10 で `capture.go` を build 除外、`supervisor.go` の `CameraTracker` 依存を剥離、ZeroMQ / `zmq.h` を廃止。Go 側は localhost TCP JSONL 受信 |
| Phase 3 | （pure Go のみ） | — | 影響なし |

→ `internal/camera/` 配下は `//go:build camera` でガード。**Phase 1 ビルドには Phase 2 依存不要**。

---

## 3. 機能マッピング（元 → Go）

| 元（React） | 移植先（Go） | 備考 |
|---|---|---|
| `src/app.jsx`（マウス追従） | `internal/mouse/follow.go` | ロジックそのまま移植。`smoothing` → `responsiveness` 改名 |
| `src/talk-app.jsx`（マイク口パク） | `internal/audio/{capture,level,envelope}.go`（malgo 完結） | Web Audio API → malgo Duplex、`getFloatTimeDomainData` → RMS 手計算（F32 [-1,1]） |
| `src/talk-app.jsx`（音声ファイル） | `internal/audio/fileplayback.go`（malgo 完結） | Ebitengine `audio/{mp3,wav,vorbis}` を **PCM デコーダーとして** 利用 + malgo Duplex で再生（同一 PCM を RMS 解析） |
| `src/talk-app.jsx`（70ms ヒステリシス） | `internal/audio/envelope.go` | `lastSwitch` を `time.Time` で管理 |
| `src/app.jsx`（自動まばたき） | `internal/blink/scheduler.go` | 確率分布そのまま（二度瞬き 22% / ゆっくり 6% / 通常 72%） |
| `src/character-config.js` | `internal/character/config.go` + YAML (camelCase) | `basePath`, `ext`, `rows`, `cols`, `sheets: { eyesOpen: { close, half, open }, eyesClosed: { ... } }` を struct に。YAML tag も camelCase で元 `character-config.js` と完全互換 |
| `public/slices2/{A-F}/r{row}c{col}.{ext}` | `internal/character/atlas.go` | 同じパスをそのまま読める (1 sheet 4500×4500px、1 cell 900×900px、component mode 切り出し前提) |
| `src/tweaks-panel.jsx` | `internal/tweaks/panel.go` | ebitenui で書き直し |
| `tools/slice_character_sheets.py` (component mode + --remove-gray-residue) | **GoTuber にも同梱**（`tools/`、完全互換） | 元 MIT 継承、ffmpeg/ffprobe 必須 |
| `Zen Maru Gothic` (Google Fonts CDN) | `internal/tweaks/assets/fonts/GenInterfaceJP-Regular.ttf` を `//go:embed` | オフライン動作可能 |

---

## 4. アーキテクチャ

### 4.1 ディレクトリ構成

```text
GoTuber/
├── cmd/
│   └── gotuber/
│       └── main.go              # エントリポイント
├── internal/
│   ├── app/
│   │   ├── app.go               # ebiten.Game 実装
│   │   └── state.go             # アバター状態
│   ├── character/
│   │   ├── config.go            # YAML 読み込み + 起動時バリデーション
│   │   └── atlas.go             # 5x5x6 スプライト（**遅延デコード + 1 シート プリロード**）
│   ├── audio/                   # malgo 完結
│   │   ├── capture.go           # malgo Duplex デバイス起動・停止
│   │   ├── level.go             # RMS 計算（F32 [-1,1]）
│   │   ├── envelope.go          # アタック/リリース + 70ms ヒステリシス
│   │   └── fileplayback.go      # 音声ファイル再生（malgo Duplex 出力、mp3/wav/ogg PCM は Ebitengine audio/{mp3,wav,vorbis} でデコード）
│   ├── camera/                  # Phase 2、`//go:build camera`
│   │   ├── capture.go           # Phase 2.10 で ignore 化 (旧 frame publisher の名残)
│   │   ├── mpclient.go          # localhost TCP 5556 detection JSONL receiver
│   │   └── mapper.go            # yaw/pitch → 5×5 cell, EAR → eyesClosed
│   ├── avatar/
│   │   ├── state.go             # cell / mouth / blink
│   │   └── draw.go              # アクティブフレーム描画
│   ├── mouse/
│   │   └── follow.go            # target/current + smoothing
│   ├── blink/
│   │   └── scheduler.go         # 不規則・二度・ゆっくり瞬き
│   ├── tweaks/
│   │   └── panel.go             # ebitenui ベースのパネル + CJK フォント
│   ├── killswitch/
│   │   └── signal.go            # OS シグナルハンドル (Phase 1.14 で簡素化: Windows では no-op)
│   └── vmc/                     # Phase 3
│       └── client.go            # OSC over UDP（自前 or go-osc）
├── assets/
│   ├── characters/
│   │   └── _default/            # プレースホルダ 5x5x6 フレーム
│   └── fonts/
│       └── NotoSansCJKjp-Regular.otf  # go:embed
├── config/
│   └── default.yaml             # 既定キャラクター設定
├── tools/
│   ├── slice_character_sheets.py # 元と同一仕様（コピー + MIT attribution）
│   ├── requirements.txt          # ffmpeg/ffprobe（外部バイナリ、Python パッケージ依存なし）
│   └── LICENSE-third-party       # 依存ライブラリのライセンス
├── docs/
│   ├── PLAN.md                  # 本ファイル
│   ├── ARCHITECTURE.md          # 詳細設計
│   ├── PHASE1.md                # Phase 1 実装ログ
│   ├── PHASE2.md                # Phase 2 実装ログ
│   └── PHASE3.md                # Phase 3 実装ログ
├── scripts/
│   ├── build.ps1
│   ├── build.sh
│   ├── dev.ps1
│   ├── dev.sh
│   # Phase 2: tools/setup-mp.{ps1,sh} で Python 3 + mediapipe + opencv-python + numpy をインストール
├── go.mod
├── go.sum
├── README.md
└── LICENSE
```

### 4.2 データフロー

```text
[入力ソース]
  ├─ マウス (ebiten.CursorPosition)
  ├─ マイク (malgo Duplex 入力 → RMS, F32 mono [-1,1])
  └─ 音声ファイル (Ebitengine audio/{mp3,wav,vorbis} で PCM デコード → malgo Duplex 出力 + 解析)
  └─ カメラ (mp_server.py Python プロセス → localhost TCP JSONL → camera.MPClient + camera.Mapper)  [Phase 2]

↓ 60 Hz Update

[Engine 層]
  ├─ mouse.Follow.Update   : current ← lerp(target, current, responsiveness)
  ├─ audio.Engine.Level    : rms  ← mic samples OR file samples（F32 mono, [-1, 1] の max）
  ├─ audio.Envelope.Update : env  ← attack(0.6) | release
  ├─ audio.MouthState      : m    ← thHalf / thFull
  ├─ blink.Scheduler.Tick  : b    ← random distribution
  ├─ camera.MPClient.Recv  : dr  ← latest detection JSON (goroutine, channel buffer 1) [Phase 2]
  └─ camera.Mapper.Map     : yaw/pitch → cell, EAR → eyesClosed [Phase 2]

↓

[avatar.State]
  ├─ cell {r, c}  ← camera.Mapper.Map(dr) (Phase 2) / mouse.Follow.Cell() (Phase 1 fallback)
  ├─ mouth        ← audio.MouthState (Phase 1.7 / Phase 2 も継続) / camera.MAR (Phase 2.5+)
  ├─ blink        ← blink.State (camera_enabled == false のみ) / camera.eyeEAR (Phase 2)
  └─ sheet        ← (blink ? 3 : 0) + mouth

↓

[avatar.Draw] — アクティブ 1 枚だけ screen.DrawImage
  activeFrame := atlas[sheet][cell]  // 遅延デコード
```

### 4.2.1 パフォーマンス予算

60 FPS = 16.67 ms/frame を以下の通り配分する。

| 処理 | 予算 | 備考 |
|---|---|---|
| mouse.Follow.Update | < 0.1 ms | 純粋な lerp 計算 |
| audio.Engine.Level | < 0.1 ms | 読み取りのみ、callback 側で RMS 計算済み |
| audio.Envelope.Update | < 0.1 ms | アタック/リリース |
| blink.Scheduler.Tick | < 0.05 ms | 確率分布 |
| avatar.State 構築 | < 0.05 ms | インデックス計算のみ |
| avatar.Draw | < 8 ms | 透過合成 + 1 枚 blit |
| OS / Ebitengine 内部 | ~6 ms | VSync 含む |
| スラック | ~2 ms | バースト対応 |

**Phase 2 顔検出**はメインループと別 goroutine で実行し、channel 経由で最新結果のみ反映（Phase 2.7 参照）。メインループ 16.67 ms 予算は維持される。

**Phase 2 配信中可用性方針 (2026-06-22 確定)**: MediaPipe サイドカー / CameraTracker がクラッシュしてもメイン GoTuber プロセスには一切影響なし (panic recover 済み、goroutine 隔離済み)。supervisor loop が exponential backoff (1s → 30s 上限) で自動再起動、5 回連続失敗で「Camera Down — Manual Restart Required」を Tweaks に表示。tracker 再起動成功で camera モード自動復帰 (配信者の手動介入ゼロ)。詳細は [PHASE2.md Section 1.1](./PHASE2.md#11-配信中可用性方針-2026-06-22-確定) 参照。

| Phase 2 追加処理 | 予算 | 備考 |
|---|---|---|
| camera.MPClient.Recv (goroutine) | 15〜50 ms | 非同期。最新値のみ channel に流す（block しない） |
| head pose + mapping (メインループ内) | < 0.5 ms | 5×5 cell 変換 + state 反映 (Phase 2 では MediaPipe 出力 JSON を cell index へ写像するだけ) |
| channel バッファ | 1 | 最新値で常に上書き、古いフレームは drop |

goroutine が遅れてもメインループに波及しない設計（Phase 2.7 と整合）。

### 4.3 コア型

```go
// internal/avatar/state.go
type Cell struct{ R, C int }                      // 0..4

type MouthState int
const (
    MouthClosed MouthState = 0
    MouthHalf   MouthState = 1
    MouthOpen   MouthState = 2
)

type Sheet int
const (
    SheetEyesOpenClosed  Sheet = 0 // A
    SheetEyesOpenHalf    Sheet = 1 // B
    SheetEyesOpenOpen    Sheet = 2 // C
    SheetEyesClosedClosed Sheet = 3 // D
    SheetEyesClosedHalf   Sheet = 4 // E
    SheetEyesClosedOpen   Sheet = 5 // F
)

type State struct {
    Cell  Cell
    Mouth MouthState
    Blink bool
    Sheet Sheet
}
```

### 4.4 設定ファイル (`config/default.yaml`)

`src/character-config.js` と完全互換の camelCase キー (Phase 1.12 で port)：

```yaml
# src/character-config.js と完全互換のキー名
basePath: "assets/characters/_default"
ext: "webp"
rows: 5
cols: 5
sheets:
  eyesOpen:
    close: "A"
    half:   "B"
    open:   "C"
  eyesClosed:
    close: "D"
    half:   "E"
    open:   "F"

tweaks:
  follow_range: 340
  responsiveness: 0.3
  char_size: 64
  bg_color_palette:
    - "#FFF8EE"
    - "#FDEFEF"
    - "#EEF4FB"
    - "#2B2926"
  bg_color: "#FFF8EE"
  mic_gain: 1.6
  th_half: 0.07
  th_full: 0.2
  release: 0.12
  auto_blink: true
  audio_file_path: ""

window:
  title: "GoTuber"
  width: 800
  height: 600
  transparent: true
  always_on_top: true
  click_through: true
  # kill_switch_key / kill_switch_rightclick: Phase 1.14 で削除予定
  # 終了はウィンドウの閉じる X ボタンで行う (Ebitengine デフォルト + OS 標準)

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

### 4.5 起動時バリデーション（`internal/character/config.go`）

YAML 読み込み後、**フェイルファスト** で以下を検証：
- `basePath` を `filepath.Abs` + `filepath.Clean` で解決済み確認（シンボリックリンクは OS 既定で追跡）
- 6 つのシートディレクトリ（A〜F）が全て存在する
- 各シートに 25 枚（5×5）の画像ファイルが存在する
- `ext` が "webp" または "png"

失敗時は `MessageBox` (Win) / `zenity` (Linux) / `osascript` (mac) でエラー表示 → `exit 1`。

---

## 5. フェーズ計画

各フェーズの詳細設計は別ファイル参照:

- **Phase 1**: [PHASE1.md](./PHASE1.md) — Pure Go PNGTuber MVP
- **Phase 2**: [PHASE2.md](./PHASE2.md) — Camera 入力（頭の方向 + 瞬き、MediaPipe Face Landmarker）
- **Phase 3**: [PHASE3.md](./PHASE3.md) — VMC Protocol 出力

| Phase | 内容 | 期間 | 状態 |
|---|---|---|---|
| 1 | Pure Go PNGTuber MVP | 1〜2 週 | 着手準備完了 |
| 1.13a | マイク選択 + TOML 永続化 | 4-5 日 | 🔜 予定 (Phase 1.13b 後) |
| 1.13b | UI 非表示ショートカット (`Ctrl+Shift+H`) | 1 日 | 🔜 予定 (1.13a の前) |
| 2 | Camera 入力 | 2〜3 週 | **確定 (2026-06-17) / 2026-06-23 改訂**: MediaPipe Face Landmarker 即採用 (Python サイドカー + localhost TCP JSONL)。Phase 1 ビルドサイズ不変 |
| 2.10.2 | Python sidecar 完全自動化 | 0.5〜1 日 | ✅ 実装完了: `.venv-mp` 自動作成 / 自動依存導入 / 自動起動 |
| 2.10.5 | 実機キャリブレーション調整 | 0.5〜1 日 | ✅ 初回反映完了: deadzone / smoothing / grace 追加 |
| 2.10.6 | transformation matrix 優先 pose 推定 | 0.5 日 | ✅ 実装完了: solvePnP fallback 化 |
| 2.10.7 | yaw ミラー補正 | 0.1 日 | ✅ 実装完了: 左右向きの体感一致 |
| 3 | VMC Protocol 出力 | 1〜2 週 | 未着手 |

各フェーズのゴール・実装項目・DoD・工数等の詳細は対応するファイル参照。

---

## 6. ビルド & 開発

### 6.1 必要環境

| ツール | バージョン | 用途 |
|---|---|---|
| Go | **1.25+** | Ebitengine v2.9 + x/image v0.42 要件 |
| C コンパイラ | MSVC（Windows）または gcc（WSL） | CGo（malgo） |
| WebView2 | 不要 | Ebitengine は Metal/DirectX/OpenGL 直利用 |
| Phase 2: Python 3 | 3.9+ | mediapipe 0.10.14+ 要件 (mp_server.py サイドカー) |
| Phase 2: IPC | localhost TCP JSONL | Python sidecar → GoTuber detection 通信 |
| Python 3 (スライス) | `pip install -r tools/requirements.txt` | スライス生成時のみ |

### 6.2 ビルドコマンド

#### Phase 1（camera なし）

```bash
# WSL Ubuntu（推奨：KASOU 用 Linux バイナリ生成）
cd /mnt/d/GitHub/GoTuber_ws/GoTuber
CGO_ENABLED=1 go build -ldflags "-s -w" -o bin/gotuber ./cmd/gotuber

# Windows native（PowerShell、ローカル確認用）
$env:CGO_ENABLED = "1"
go build -ldflags "-s -w" -o bin/gotuber.exe .\cmd\gotuber

# WSL から Windows へクロスコンパイル
GOOS=windows CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc \
  go build -ldflags "-s -w" -o bin/gotuber.exe ./cmd/gotuber

# テスト
go test ./... -v -race
```

#### Phase 2（camera 有効）

> **✅ Windows native camera build は成立** (Phase 2.10 完了)。
> `blackjack/webcam` / ZeroMQ (`zmq.h`) / `CameraTracker` 直依存は除去済み。
> `build.ps1 -Camera` は `bin/gotuber-camera.exe` を生成する。

```bash
# Linux native (WSL Ubuntu)
cd /mnt/d/GitHub/GoTuber_ws/GoTuber
CGO_ENABLED=1 go build -tags camera -ldflags "-s -w" \
  -o bin/gotuber-camera ./cmd/gotuber

# Windows native
.\scripts\build.ps1 -Camera

# Python サイドカー環境セットアップ
pip install -r tools/requirements-mp.txt

# mp_server.py 起動 (GoTuber.exe から自動で spawn される、デバッグ時は手動起動)
python tools/mp_server.py

# カメラ無効で起動 (Phase 1 マウスモードのみで動作)
./bin/gotuber --no-camera
```

### 6.3 開発ループ

```bash
# WSL: go run で即時再起動
go run ./cmd/gotuber --character assets/characters/_default

# ログレベル
GOTUBER_LOG_LEVEL=debug go run ./cmd/gotuber

# KASOU へデプロイ (将来用、Phase 1 では未実装)
# KASOU_HOST=kasou ./scripts/deploy-kasou.sh   # .ssh/config に Host kasou エントリ要
```

### 6.4 緊急時の KASOU クリーンアップ

- ~~`Esc` 押下 or Ctrl+C（SIGINT）でクリーン終了~~ → **Phase 1.14 で削除予定**。終了は **ウィンドウの閉じる X ボタン** (全 OS graceful) または **Ctrl+C** (Unix: `signal.Notify` 経由 graceful / Windows: Go runtime デフォルト即終了)
- クリックスルーでクリック不能になっても SSH ログインして `pkill -f gotuber`

---

## 7. リスク & 対策

| # | リスク | 影響 | 対策 |
|---|---|---|---|
| R1 | CGo によるクロスコンパイル不可 | 中 | 各 OS で個別ビルド。CI は GitHub Actions matrix |
| R2 | MediaPipe セットアップ摩擦（Python サイドカー） | 中 | Phase 2 開始時に README + `tools/requirements-mp.txt` 整備、起動失敗時は Phase 1 マウスモードで graceful degradation。配信中の可用性確保は supervisor loop (PHASE2.md 1.1) で担保、5 回連続失敗で manual restart 待ち |
| R3 | 透過ウィンドウの OS 差異 | 中 | Issue #3222 回避手順を Phase 1.2 に明記。Windows は `WS_EX_TRANSPARENT` フォールバック |
| R4 | Mic 権限の UX | 中 | OS ごとの初回ダイアログ。エラー時は audio 無効で続行 |
| R5 | 起動時 150 枚のデコード時間 | 低 | **遅延デコード** で起動時は 1 シートのみ。目標 < 500 ms（実測） |
| R6 | Python サイドカー依存追加 | 低 | GoTuber.exe は Go バイナリのみで変化なし、Phase 2 利用時に mediapipe/opencv-python/numpy が必要 (~145MB) |
| R6.1 | Python 依存インストール時の高 CPU / 長待ち | 中 | Windows 実機で `Preparing metadata (pyproject.toml)` 中に CPU 99% を確認。**対策**: Phase 2.10.2 で `tools/setup-mp.{ps1,sh}` を導入し、`pip/wheel/setuptools` 先行更新・`--prefer-binary`・必要なら `--no-build-isolation` を固定してビルド負荷を緩和する |
| R7 | VMC Protocol 仕様の網羅性 | 中 | Phase 3 は最小実装（blend shapes のみ） |
| R8 | Ebitengine の将来の破壊的変更 | 低 | v2.9.9 stable に固定、v3 alpha は本番採用しない |
| R9 | アセット権利問題 | 中 | 元画像は同梱しない。`tools/slice_character_sheets.py` 同梱で自前生成運用 |
| R10 | go-osc メンテナンス停止（2022-03 以降） | 中〜高 | Phase 3 開始時に分枝 or 代替検討。自前 UDP 実装（~150 行）も候補 |
| R11 | クリックスルー有効時のロックイン | 中 | Phase 1.14 で kill switch (Esc / signal.Notify on Windows) 削除 → 対策: ウィンドウ X ボタン (全 OS graceful) + Ctrl+C (Unix: signal.Notify 経由 graceful / Windows: Go runtime デフォルト即終了) + OS タスクバー / Alt+Tab |
| R12 | フォーカスフリッカー（クリックスルー有効化直後 200ms） | 中 | xdotool で KASOU 検証、60 フレーム遅延発火オプションで対応 |
| R13 | Phase 2 tracker クラッシュ時の可用性 | 中 | CameraTracker の defer recover で panic 吸収、supervisor loop が exponential backoff で自動再起動 (1s → 30s 上限、3 回成功でリセット)、5 回連続失敗で Tweaks に "Camera Down — Manual Restart Required" 表示。実装: `internal/camera/supervisor.go` (Phase 2.5) |
| R14 | **Phase 2 Windows native camera runtime 差異** | 中 | Phase 2.10 で build blocker (`blackjack/webcam`, `zmq.h`, `CameraTracker` 直依存) は除去済み。ただし Python sidecar 実行環境（venv, OpenCV camera access, MediaPipe model path, localhost TCP 接続）で OS 差異が残る。**対策**: `build.ps1 -Camera` / `dev.ps1 -Camera` / `tools/setup-mp.ps1` を基準運用に統一し、visual test で Lost Signal / Down / Restart を確認。詳細は [PHASE2.md Phase 2.10](./PHASE2.md#phase-210-windows-native-camera-対応-2026-06-23--未着手) |

---

## 8. 想定バイナリサイズ

| Phase | 依存 | 想定サイズ |
|---|---|---|
| Phase 1 | Ebitengine v2.9.9 + malgo v0.11.25 + ebitenui + webp v0.42 | **8〜15 MB** |
| Phase 2 | + mediapipe 0.10.14+ (Python プロセス別) | **Go バイナリ変化なし (19 MB)**、Python env +125 MB |
| Phase 3 | + 自前 OSC（go-osc を避ける場合） | +0.1 MB |

upx 圧縮でそれぞれ 30% 削減可。

### 8.1 メモリ予算

| 項目 | 想定 |
|---|---|
| 150 フレーム全展開（最悪ケース） | 864 MB (150 × 1200×1200 × 4 byte RGBA) |
| 1 シート（25 枚）+ 近傍予備プリロード | **~120 MB** |
| 起動時プロセスメモリ（OS 含む） | **40〜80 MB** |
| 24/7 連続運用時の RSS 目標 | **< 150 MB** |

**方針**: アトラスは **遅延デコード**（avatar.Draw で初めて参照されたフレームをデコード）。よく使う 1 シート + 近傍をプリロード（Section 4.1 `atlas.go`）。

### 8.2 エラー UX

- **起動時致命的**（画像不足等）: `MessageBox` (Win) / `zenity` (Linux) / `osascript` (mac) → `exit 1`
- **実行時非致命的**（mic 切断等）: ステータスバーアイコン + 5 秒後 Tweaks 警告
- **実行時致命的**（mp_server.py クラッシュ / TCP 検出ストリーム切断 等）: ウィンドウ閉じる X ボタンで安全停止 + ログ（Phase 1.14 で仕様確定）、起動失敗時は graceful degradation で Phase 1 マウスモードへフォールバック

---

## 9. 元プロジェクトとの互換性

- 同じスライス画像（`{base}/{A-F}/r{row}c{col}.{ext}`）を参照可能
- `character-config.js` → `config.yaml` で同じ構造を宣言
- Tweaks の値の意味はほぼ同一（`smoothing` → `responsiveness` のみ改名）
- アセットライセンスは元のまま（**再配布不可**）。エンドユーザーが自分で用意
- `tools/slice_character_sheets.py` を GoTuber にも同梱（MIT、attribution 付き）

---

## 10. 想定ディレクトリ（ビルド成果物）

```text
GoTuber/
├── bin/
│   ├── gotuber-linux-amd64
│   ├── gotuber-windows-amd64.exe
│   └── gotuber-linux-amd64-camera       # Phase 2: -tags camera ビルド (-tags なしビルドでも動作)
├── dist/
└── ... (上記 4.1 構成)
```

---

## 11. 未解決事項（実装前確認）

実装着手前に以下の判断が必要。**推奨デフォルトで良ければ「ぜんぶ OK」**。

- [x] **Q1**: デフォルト同梱キャラクターの調達元
  - 案B: アセット同梱せず、ユーザーが `tools/slice_character_sheets.py` で生成
  - **決定: 確定（案B 採用）**。Phase 1.1 で `tools/slice_character_sheets.py` を `tools/` に配置、ユーザー各自でアセット生成

- [x] **Q3**: クリックスルー実装方式
  - 案A: Ebitengine `SetWindowMousePassthrough` を `Update()` 初回で呼ぶ
  - 案B: Win32 `WS_EX_TRANSPARENT` フォールバック
  - **決定: 確定（案A 採用）**。Phase 1.2 で `Update()` 初回呼び出し。失敗時は Phase 1.3 で案B へフォールバック

- [x] **Q4**: Python スライスツール同梱
  - **決定: 確定（Yes）**。MIT 継承の `tools/slice_character_sheets.py` を GoTuber に配置、依存ゼロ CLI として独立運用

- [x] **Q6**: 音声ファイル再生 in Phase 1
  - **決定: 保留**。Phase 1 はマイクのみに集中（メインはメインマイクで同時 Realtime 口パク）。**2026-06-17 更新**: Phase 1 公開リリース後の post-release 扱いに変更、公開リリースまでにユーザーニーズがなければ実装しない。malgo Duplex と Ebitengine audio/{mp3,wav,vorbis} は post-release で再評価。詳細 → `docs/post_release.md` Section 1.5

- [x] **Q8**: Phase 2 顔モデル
  - **2026-06-17 確定 / 2026-06-23 改訂**: **MediaPipe Face Landmarker (Tasks API) 即採用**。Python サイドカー構成 (localhost TCP JSONL IPC) で Go 側に mediapipe / opencv の import なし。YuNet は不採用 (頭の方向だけなら YuNet で十分だが、まばたき EAR も取りたいので 478 landmarks の MediaPipe に統一)。Phase 1 ビルドサイズ・依存は変化なし (GoTuber.exe は Go バイナリのみ、Python は別プロセス)。詳細は [PHASE2.md](./PHASE2.md) §4

- [x] **Q12 (新規)**: 配布チャネル
  - 案A: GitHub Releases のみ（手動ダウンロード）
  - **決定: 確定（案A 採用）**。Phase 1 は GitHub Releases 手動ダウンロードのみ。自動更新は需要が出たら Phase 4 以降

- Q2（ビルド環境）/ Q5（パッケージング）/ Q7（フォント）/ Q9（macOS 権限）/ Q10（CI）/ Q11（署名）は v0.3 で方針確定済み。

---

## 12. 次のアクション（即着手可能）

Phase 1 を以下の順序で進める。各ステップ完了時にコミット + 動作確認ログを `docs/PHASE1.md` に追記する。

1. **Phase 1.1**: WSL Ubuntu で `go mod init github.com/<owner>/GoTuber`、Ebitengine v2.9.9 + malgo v0.11.25 + ebitenui + webp を `go get` で導入、最小 main.go（空ウィンドウ）
2. **Phase 1.2**: 透過ウィンドウ + クリックスルー（`Update()` 初回）+ KASOU で実機確認（xdotool フォーカスフリッカー検証含む）
3. **Phase 1.3**: スプライトアトラス loader（1 シート プリロード + 遅延デコード、`image.RegisterFormat` で webp 登録、Loading 表示）
4. **Phase 1.4**: 設定 YAML 読み込み + 起動時バリデーション（4.5）
5. **Phase 1.5**: マウス追従（`responsiveness` 改名）+ テスト
6. **Phase 1.6**: 自動まばたき + テスト
7. **Phase 1.7**: malgo マイク + Ebitengine 音声ファイル + エンベロープ + 口パク + テスト
8. **Phase 1.8**: Tweaks パネル（ebitenui）+ CJK フォント埋め込み（英語ラベル）
9. **Phase 1.9**: ビルドスクリプト（`build.ps1` / `build.sh` / `dev.*`）+ Windows + Linux 動作確認
10. **Phase 1.10**: README + LICENSE + `tools/requirements.txt` + `tools/LICENSE-third-party` + `docs/PHASE1.md` + **`go test ./...` 全パス確認**
11. **Phase 1.11**: Polish 適用 (decodePCM16 sync.Pool、Slider 定数化、clampInt) + **`tools/slice_character_sheets.py`** 実装 (5×5 シート → 25 枚分割、MIT 継承) + テスト 8 件
12. **Phase 1.12**: キャラクターシステム **全 port (tomari-guruguru → Go)** — Phase 1.11 の自作 (snake_case キー・Y軸反転・シンプル版スライスツール) を **完全廃止** し、元 `src/character-config.js` の camelCase スキーマ (`basePath`, `eyesOpen`, `eyesClosed`, `close`)、元 `src/app.jsx:60-62` の Y軸反転なし計算式、元 `tools/slice_character_sheets.py` (648 行・component mode・`--remove-gray-residue`) を MIT 継承。設定 YAML も camelCase 化。マウス追従は Y 軸反転削除 (`r0=上, r4=下`)。`docs/新キャラ差し替え手順.md` は元 MIT 翻訳を全面書き換え (「100% port」を冒頭明記)
13. **Phase 1.13a** (予定): マイク選択 + TOML 永続化 — malgo `Devices` 列挙 → ebitenui `ListComboButton` (ComboBox) ドロップダウン → **選択デバイスの malgo 内部 ID** を `os.UserConfigDir() + "GoTuber/config.toml"` (`[audio] device_id = "..."`) に保存 → 再起動時復元 (ID で照合するため表示名重複も問題なし)。新規パッケージ: `internal/config/` (Load/Save, `os.UserConfigDir()` でパス取得), `internal/audio/devices.go` (ListDevices/NewCaptureByID)。既存 `internal/audio/capture.go` をデバイス ID 対応に拡張。サブフェーズ 1.13a.1〜1.13a.8 (4-5 日)。詳細は `docs/PHASE1.md` Section 10.2
14. **Phase 1.13b** (予定): UI 非表示ショートカット — `Ctrl+Shift+H` で Tweaks パネル + Settings ボタン + 設定ドロップダウン全部を**トグル**表示/非表示。OBS ウィンドウキャプチャで UI が映り込まないようにする。~~kill switch (Esc) は uiHidden に関わらず常に有効~~ → **Phase 1.14 で削除予定**。新規 `Game.uiHidden bool` フィールド。サブフェーズ 1.13b.1〜1.13b.6 (0.9 日、1.13b.4 kill switch 確認は Phase 1.14 に移動)。**1.13b を先にリリース** (リスク小・独立性高)、その後 1.13a 着手。詳細は `docs/PHASE1.md` Section 10.3
15. **Phase 1.14** (予定): 終了ショートカット削除 + F1 表示時 audio restart crash 修正 — `Esc` / `Q` キー検出と `killswitch.Install()` の Windows 限定削除 (`runtime.GOOS == "windows"` ガード)。**Unix では `signal.Notify` を維持** (Ctrl+C → graceful 終了)。**Windows では `signal.Notify` 削除** (Ctrl+C は Go runtime デフォルト即終了、推奨は X ボタン)。終了は **ウィンドウの閉じる X ボタン** (全 OS graceful) または **Ctrl+C** (Unix: graceful / Windows: 即終了)。Phase 1.13 visual test で F1 / Esc 押下時にアプリが即終了する致命バグ発覚。追加実機ログで F1 直後に `config saved: audio.device_id = ""` が出るため、F1 直接終了ではなく **F1 → panel 表示 → ComboBox 初期選択 → `Mover.Restart("")` → audio capture lifecycle 破壊** が最有力。`NewCaptureByID()` の success/error cleanup 分離、ComboBox 初期選択・同一 device ID 選択時の save/restart スキップも含める。サブフェーズ 1.14.0 (真因調査) 〜1.14.8 (1.9 日)。詳細は `docs/PHASE1.md` Section 11
16. **Phase 1.14.9** (予定): X ボタン通過バグ fix — `internal/game/game.go` の `firstUpdate` ブロックが空だったため、Ebitengine v2 + `ScreenTransparent:true` のデフォルト passthrough (`true`) に依存していた。F1 押下後の `SetWindowMousePassthrough(false)` の効果が Ebitengine v2 GLFW バックエンドで遅延するケースがあり、X / 最小化 / 最大化ボタンクリックが通過する。`firstUpdate` で明示的に `applyPassthrough()` を 1 回呼ぶことで初期状態を確定 (Phase 1.2 と同じパターン)。回帰点は `508f630` Phase 1.13a/b で `applyPassthrough()` 導入時に `firstUpdate` から直接 `SetWindowMousePassthrough(true)` 呼び出しが消えたこと。詳細は `docs/PHASE1.md` Section 11.9
17. **Phase 1.14.14** (実装): adaptive noise gate — yosia さん実機で「全マイクを試しても口パクしない」症状 (`RMS 0.0038 / Envelope 0.0091 / Mouth closed`) に対応。固定閾値 (0.05 / 0.20) では実機マイクの RMS レンジに合わないため、`noiseFloor` を毎フレーム exponential filter で学習 + gate ヒステリシス (open 0.002 / close 0.001) + sensitivity ゲイン。起動 60 frame の warmup で gate 即開き回避。`UpdateWithMetrics()` 戻り値を tuple → `Metrics` 構造体 (6 フィールド) に変更。Tweaks debug を 2 行化。詳細は `docs/PHASE1.md` Section 13
18. **Phase 1.14.15** (実装): Mic Sensitivity slider — Phase 1.14.14 visual test で、無音ノイズ `RMS 0.0084 / Floor 0.0020` が 15x で `Gated 0.081` となり `MouthHalf` 閾値を超えることを確認。`defaultMicSensitivity` を 15.0 → 10.0 に下げ、Tweaks に 1.0x..20.0x (0.1x 刻み) の slider を追加。TOML 永続化は未実装、まず visual test で適切値を探る。詳細は `docs/PHASE1.md` Section 14
19. **Phase 1.14.16** (予定): Tweaks 永続化 (明示的 Save ボタン方式、Reset 機能 YAGNI 削除) — yosia さん実機で「Mic 以外が全部再起動で消える」症状発覚。Tweaks パネルに `Save` ボタンと `unsaved changes` status を追加。スライダー・チェックボックス変更時は `state.Dirty = true` を立てるだけで TOML には書かない、Save 押下時にまとめて 1 回書き込む。TOML は `[audio] device_id` + `[tweaks] mouse_responsiveness / blink_enabled / mouth_enabled / mic_sensitivity` の 5 フィールド。`BlinkEnabled` / `MouthEnabled` は `*bool` で TOML キー欠落 (nil) と false 区別。`PanelVisible` / `uiHidden` / audio debug 6 フィールドは揮発。Mic デバイスは Save ボタン待たず即時保存 (Phase 1.13a 互換)。**Round 3 で Reset ボタンを YAGNI 削除**: ebitenui slider の `lastCurrent` が private で外部更新不可、`ChangedEvent.Fire()` 後 `Render()` が `s.Current != s.lastCurrent` で再 fire するため、`RefreshWidgetsFromState()` 系の根本修正は ebitenui upstream PR / fork が必要。デフォルトに戻したい場合は TOML ファイル削除で対応。Phase 1.13a で導入済みの `config.Load/Save` を再利用、新規パッケージなし。詳細は `docs/PHASE1.md` Section 15

---

## 13. 関連ドキュメント

- 元プロジェクト: `../tomari-guruguru/README.md`
- 元の口パクエンジン: `../tomari-guruguru/src/talk-app.jsx`
- 元のまばたき: `../tomari-guruguru/src/app.jsx`（74〜110行目）
- スライス画像仕様: `../tomari-guruguru/docs/新キャラ差し替え手順.md`
- VMC Protocol 仕様: https://protocol.vmc.info/english.html
- Ebitengine v2.9.9: https://pkg.go.dev/github.com/hajimehoshi/ebiten/v2（**Go 1.25+**）
- Issue #3222 (transparent window): https://github.com/hajimehoshi/ebiten/issues/3222
- malgo v0.11.25: https://github.com/gen2brain/malgo
- MediaPipe Face Landmarker (Phase 2): https://developers.google.com/mediapipe/solutions/vision/face_landmarker
- golang.org/x/image/webp v0.42.0: pkg.go.dev/golang.org/x/image/webp（**Go 1.25+**）
- go-osc（Phase 3 候補）: https://github.com/hypebeast/go-osc（最終更新 2022-03、R10 参照）

---

*v0.4.6 改訂: Phase 2 配信中可用性方針追加 (Section 1.1 確定)。MediaPipe tracker クラッシュ時もメイン GoTuber 影響なし、supervisor loop が exponential backoff で自動再起動 (1s→30s、5 回連続失敗で manual restart 待ち)、R13 リスク追加。v0.4.5 Phase 2 MediaPipe 即採用、Q8 解除から継続。Phase 1 ビルドサイズ・依存不変。*
