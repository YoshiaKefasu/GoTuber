# GoTuber

> **軽量 PNG アバターアプリ** — Windows 優先、Golang 製、OBS 透過キャプチャ対応

[![Status](https://img.shields.io/badge/status-Phase%201%20Done-brightgreen)](#roadmap)
[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go)](#ビルド)
[![License: MIT](https://img.shields.io/badge/code-MIT-blue.svg)](LICENSE)

**19.5 MB の exe 1 個で動く透過アバターアプリ**。PNG 画像を 25 方向に動かしてアバター表示 + **メインマイクで同時 Realtime 口パク**。OBS のウィンドウキャプチャに重ねて配信やビデオ通話に使う。

前身: [tomari-guruguru](https://github.com/rotejin/tomari-guruguru) を **Golang で完全書き直し**（React/Vite 製 → 単一バイナリ）。

## 特徴

### Phase 1（MVP・全 Phase 1.1〜1.12 実装完了）
- **WebP / PNG アバター表示** — 5×5 角度 × 6 表情 = 150 枚のスプライト切替（元 tomari-guruguru 互換の 1200×1200 anchored フレーム）
- **マウス追従** — カーソルに 25 方向で振り向く（r0=上、r4=下、c0=左、c4=右、元 `app.jsx:60-62` と完全一致）
- **メインマイクで同時 Realtime 口パク** — 喋ると口の 3 段階（とじ/はんびらき/ぜんかい）が自動切替。VTuber 配信用途の主機能
- **自動まばたき** — 不規則なタイミング（自然な分布、二度・ゆっくり含む）
- **透過背景ウィンドウ** — OBS でウィンドウキャプチャするだけ（クロマキー不要）
- **クリックスルー** — キャラの背後をクリックできる（配下アプリの操作を邪魔しない）
- **Tweaks パネル** — 追従速度・自動まばたき ON/OFF・口パク ON/OFF を `F1` キーまたは Quit ボタンで操作
- **CJK フォント埋め込み** — Gen Interface JP Regular (Inter + Noto Sans JP) をバイナリ同梱
- **終了** — **ウィンドウの閉じる X ボタン** (全 OS graceful) または **Ctrl+C** (Unix: graceful / Windows: 即終了)。`Esc` / `Q` キーの kill switch は削除済み (Phase 1.14)
- **元 `src/character-config.js` 互換** — `basePath`, `eyesOpen`, `eyesClosed`, `close` の camelCase キー (Phase 1.12 で port)
- **マイクデバイス選択** (Phase 1.13a 予定) — F1 → Settings → ドロップダウンで OS 全入力デバイスから選択、`os.UserConfigDir()/GoTuber/config.toml` に保存して再起動時に復元

> **Phase 1 スコープ外**（post_release、詳細 → `docs/post_release.md` Section 1.5）: 音声ファイル口パク（mp3/wav/ogg）

### Phase 2（カメラ VTuber・MediaPipe 即採用で確定）

- **Webcam 頭の方向トラッキング** — 顔の動き（左右・上下）にキャラが追従（5×5 グリッド）
- **自動まばたき（EAR）** — MediaPipe Face Landmarker の Eye Aspect Ratio で瞬き検出
- **フェイルセーフ** — 顔が消えたら自動的にマウスモード（Phase 1.5）に切替
- **Camera ON/OFF 切替**（Phase 2.10.8 実装完了） — Tweaks からカメラ追従を止めて、mouse グルグルへ戻せる
- **Python サイドカー構成** — MediaPipe は Python プロセスで実行、GoTuber.exe は Go バイナリのみ（サイズ不変、起動失敗時は graceful degradation）
- **MediaPipe モデル同梱** — `assets/models/face_landmarker.task` を同梱（Google / MediaPipe、Apache-2.0）。通常起動時の自動ダウンロード不要
- **配信中可用性** — MediaPipe tracker がクラッシュしてもメイン GoTuber は無影響、supervisor loop が exponential backoff (1s→30s) で自動再起動、5 回連続失敗で manual restart 待ち
- 口の縦横比 (MAR) カメラ検出は Phase 2.5+ で再評価（Phase 1.7 の malgo マイクと排他利用）

### Phase 3（Creator Tools・仕様固定中）
- **1 枚入力 → A 25 枚** — 目開き + 口閉じのメイン画像から、5×5 の A 状態を生成
- **目眉 / 口マスク生成** — AI Inpaint で B〜F を作るための赤マスク PNG を出力
- **低コスト制作支援** — Live2D モデルなしで GoTuber 用キャラ素材を作りやすくする
- **Phase 3.0**: 仕様固定フェーズ（ディレクトリ構造・マスク命名・レビュー PNG 色・validate 要件を確定）
- **キャラクター名管理**: `--character <name>` で出力ディレクトリを切り替え可能（デフォルト `_default`）
- **ライセンス**: サンプルキャラクター画像（`_default`）は動作検証自由・動画/配信利用は yosia 許可必要・再配布禁止

## 競合との位置付け

| ツール | サイズ | 特徴 | 位置 |
|---|---|---|---|
| VTube Studio | 200MB+ | Live2D / 3D、物理演算、Steam 連携 | 高機能・重い |
| Veadotube Mini | ~100MB | シンプル PNGTuber | 中機能 |
| PNGTubeRemix | ~80MB | Godot 製、高カスタマイズ | 中機能 |
| tomari-guruguru（元） | Web アプリ | ブラウザ必須 | 軽量・だけど Web |
| **GoTuber** | **19.5MB** | **exe 単体・透過キャプチャ・Realtime マイク口パク** | **超軽量 + 配信者向け** |

**「Live2D じゃなくていい、PNG の 5×5 アバターで十分、かつ 1 バイナリで完結したい」人向け**。

## 配信者の使い方

```
1. GoTuber.exe をダウンロード (19.5MB)
2. exe ダブルクリックで起動
3. 自分のキャラ PNG を assets/characters/<name>/ に配置
   （or デフォルトのプレースホルダで起動）
4. OBS を起動
5. ソース追加 → ウィンドウキャプチャ → GoTuber を選択
6. 配信開始
7. 喋る → キャラの口が動く
8. (Phase 2) カメラの前に座る → 顔でキャラが動く
9. Tweaks を弄りたい時 → F1 キー
10. 終了したい時 → ウィンドウの X ボタン (推奨、全 OS graceful) または Ctrl+C (Unix: graceful / Windows: 即終了)
```

## 動作環境

| OS | 対応 | 備考 |
|---|---|---|
| **Windows 10 / 11** | ✅ 優先 | mingw-w64 ビルド / DirectX |
| **Linux (X11)** | ✅ 検証済 | gcc + libasound2-dev |
| **Linux (Wayland)** | ⚠️ 要検証 | Ebitengine Wayland サポートに依存 |
| **macOS 11+** | ✅ 対応予定 | Metal |

- WebView2 不要（pure ネイティブ描画）
- マイク・カメラは対応 OS の標準 API でアクセス
- **マイクなしでも起動可能**（口パク無効で続行）

## 開発者向け

### 必要環境

- **Go 1.25+**（Ebitengine v2.9 + `golang.org/x/image v0.42` 要件）
- C コンパイラ（MSVC または gcc）— CGo (malgo) のため
  - Windows: `scoop install mingw` または MSVC Build Tools
  - WSL: `sudo apt install gcc libasound2-dev build-essential`
- Phase 2 以降: MediaPipe Face Landmarker (Python サイドカー) + localhost TCP JSONL IPC
  - Face Landmarker モデルは `assets/models/face_landmarker.task` に同梱済み（約 3.6 MB、Apache-2.0）
- Python 3（スライス生成時のみ、純粋な標準ライブラリのみ使用、ffmpeg/ffprobe が必要）— `pip install -r tools/requirements.txt`

### ビルド

#### Windows native
```powershell
.\scripts\build.ps1            # リリースビルド (19.5 MB)
.\scripts\build.ps1 -Dev       # デバッグビルド
.\scripts\build.ps1 -Clean     # bin/ 削除してビルド
```

#### Camera build (Phase 2)
```powershell
.\scripts\build.ps1 -Camera      # Windows native camera build
.\bin\gotuber-camera.exe
```

補足:
- Phase 2.10 で `blackjack/webcam` / ZeroMQ 依存を除去済み
- Go 側 camera 通信は localhost TCP JSONL
- Python sidecar 実行時は `tools/requirements-mp.txt` の依存が必要
- Tweaks の **Camera Enabled** トグルで camera supervisor の起動/停止を制御可能 (Phase 2.10.8)

#### WSL Ubuntu / Linux
```bash
./scripts/build.sh             # リリースビルド
./scripts/build.sh --dev       # デバッグビルド
./scripts/build.sh --skip-test # テストスキップ
./scripts/build.sh --camera    # Phase 2 camera 有効ビルド
```

#### 開発ループ（ビルド + 実行）
```powershell
.\scripts\dev.ps1              # Windows
.\scripts\dev.ps1 -Camera     # camera 有効 (Windows native)
```
```bash
./scripts/dev.sh               # Linux
```

#### テスト
```bash
go test ./...                  # 全パッケージ
go test ./... -v -race         # verbose + race detector
```

## 使い方

### 起動

`bin/gotuber.exe`（Windows）または `bin/gotuber`（Linux）をダブルクリック / 実行。

### デフォルトキャラ

`assets/characters/_default/` 配下に 6 シート × 25 枚 (= 150 枚) のプレースホルダ PNG。
- `A` (eyes_open + mouth_closed) — 通常の目 + 閉じ口
- `B` (eyes_open + mouth_half)
- `C` (eyes_open + mouth_open)
- `D` (eyes_closed + mouth_closed) — 瞬き
- `E` (eyes_closed + mouth_half)
- `F` (eyes_closed + mouth_open)

各シートの画像フォーマット: `r{0-4}c{0-4}.{png|webp}`（5行 × 5列 = 25枚）

行/列の向き (元 tomari-guruguru と同じ):
- `r0` = 上を見る、`r4` = 下を見る
- `c0` = 左を見る、`c4` = 右を見る

### 自分のキャラを作る (推奨フロー)

元プロジェクト [tomari-guruguru](https://github.com/rotejin/tomari-guruguru) と完全互換のフロー。詳細手順は `docs/新キャラ差し替え手順.md` 参照。

**1. 6 枚の素材画像を用意** (各 4500×4500px、5×5 グリッド、1 コマ 900×900px):

| シート | 目 | 口 | ファイル名例 |
|---|---|---|---|
| A | 開け | とじ | `A_目開け_口とじ.png` |
| B | 開け | 中間 | `B_目開け_口中間.png` |
| C | 開け | 開け | `C_目開け_口開け.png` |
| D | 閉じ | とじ | `D_目閉じ_口とじ.png` |
| E | 閉じ | 中間 | `E_目閉じ_口中間.png` |
| F | 閉じ | 開け | `F_目閉じ_口開け.png` |

**2. スライスツールで component mode 切り出し** (髪のはみ出しを保持):

```bash
# インストール
pip install -r tools/requirements.txt
# ffmpeg / ffprobe（必須、全 OS）
brew install ffmpeg            # macOS
sudo apt install ffmpeg        # Ubuntu / Debian
# Windows: https://www.gyan.dev/ffmpeg/builds/ から DL し PATH に追加

# 6 シート一括スライス
python tools/slice_character_sheets.py \
  --source "新キャラ資料" \
  --slices-out "assets/characters/mychara" \
  --format webp \
  --component-mode \
  --remove-gray-residue \
  --alpha-threshold 64
```

→ `assets/characters/mychara/{A-F}/r{0-4}c{0-4}.webp` (150 枚) 出力。

**3. 設定ファイル作成** (`config/mychara.yaml`)。**`src/character-config.js` と完全互換の camelCase キー**:

```yaml
# src/character-config.js と完全互換のキー名
basePath: "assets/characters/mychara"
ext: "webp"  # デフォルト webp（元プロジェクトと同じ）
rows: 5
cols: 5
sheets:
  eyesOpen:
    close: "A"
    half: "B"
    open: "C"
  eyesClosed:
    close: "D"
    half: "E"
    open: "F"
```

**4. 起動** (Phase 1.12 時点):

```powershell
# パターン A: 既存 default.yaml の basePath を書き換える（最も簡単）
notepad config\default.yaml
# basePath: "assets/characters/mychara" に変更して保存
.\bin\gotuber.exe

# パターン B: 新しい設定ファイルを作って default.yaml と差し替える
Copy-Item config\mychara.yaml config\default.yaml -Force
.\bin\gotuber.exe
```

> **注**: Phase 1.11 までは `-config` フラグで任意 YAML を指定できる想定でしたが、
> main.go は `config/default.yaml` を直接読む実装のため、当面は default.yaml を
> 切り替える運用です。複数キャラ対応は Phase 1.12 完了後に再評価。

詳細な検証手順・コンポーネント mode の調整パラメータは [`docs/新キャラ差し替え手順.md`](docs/新キャラ差し替え手順.md) 参照。

**AI 画像生成でキャラを作る場合**: [`docs/01_画像生成用プロンプト.txt`](docs/01_画像生成用プロンプト.txt) (元 [tomari-guruguru](https://github.com/rotejin/tomari-guruguru) を MIT 継承) に ChatGPT Images 2.0 用の 5×5 顔角度リファレンス生成プロンプト + 目/口の 6 表情差分プロンプト + ファイル命名規則を収録。同梱の 5×5 テンプレート画像 [`docs/01_上半身_画像生成用テンプレ.png`](docs/01_上半身_画像生成用テンプレ.png) (4500×4500、GoTuber 作成者自作) を AI に添付して使用。

### テスト実行

```bash
# スライスツール (Phase 1.12 で元 tomari-guruguru 648 行版を MIT 継承)
# - 単体で動作する CLI、テストは元プロジェクトに存在しない
# - 使い方: python tools/slice_character_sheets.py --help
python tools/slice_character_sheets.py --help
```

### キー操作

| キー | 動作 |
|---|---|
| `F1` | Tweaks パネル表示/非表示 |
| `Ctrl+Shift+H` | 全 UI 非表示/再表示 (Phase 1.13b) |
| `Ctrl+C` | 終了（コンソールから）| Unix: graceful (`signal.Notify` 経由) / Windows: Go runtime デフォルト即終了 |
| **ウィンドウの X ボタン** | 終了 (推奨) |

> **Phase 1.14 で削除済み**: `Esc` / `Q` キーの kill switch を完全削除。代わりに **F1 で Tweaks パネル → Quit ボタン** が GUI 唯一の終了手段 (Phase 1.14.4)。X ボタン・Ctrl+C も引き続き有効。F1/Esc 即終了バグは audio lifecycle の double-free が真因 (Phase 1.14.1) と判明。

マウス操作: キャラに向かって顔を向ける（5×5 グリッド追従）。

### Tweaks パネル（F1）

- **Mouse Follow** スライダー: 追従速度（0.05 〜 1.0、デフォルト 0.3）
- **Auto Blink** チェック: 自動まばたき ON/OFF
- **Mic Mouth Movement** チェック: マイク口パク ON/OFF（マイクなし環境では無効化）
- **Quit** ボタン: アプリ終了

## プロジェクト構成

```
GoTuber/
├── cmd/gotuber/             # エントリポイント
├── internal/                # 内部パッケージ
│   ├── game/                # Ebitengine ゲームロジック
│   ├── audio/               # malgo 完結 (mic + RMS + 口パク)
│   │   ├── capture.go       # malgo mic capture
│   │   ├── envelope.go      # attack/release + ヒステリシス口パク
│   │   └── mover.go         # 高レベル facade
│   ├── character/           # アトラス + 設定 + バリデーション (Phase 1.12 で port)
│   ├── mouse/               # マウス追従 (5×5 グリッド、r0=上)
│   ├── blink/               # 自動まばたき (22/6/72 分布)
│   ├── tweaks/              # Tweaks パネル (ebitenui + CJK フォント)
│   │   ├── font.go          # //go:embed で TTF 埋め込み
│   │   ├── panel.go         # ebitenui UI
│   │   ├── state.go         # パネル状態
│   │   └── assets/fonts/    # Gen Interface JP Regular.ttf
│   └── killswitch/          # SIGINT + Esc
├── assets/
│   ├── characters/          # デフォルトキャラ + ユーザー配置先
│   │   └── _default/        # 6 シート × 25 枚 = 150 プレースホルダ (Phase 1.12 で WebP 化)
│   └── (Tweaks フォントは internal/tweaks/assets/ 配下)
├── config/
│   └── default.yaml         # デフォルトキャラ設定 (camelCase, character-config.js 互換)
├── tools/
│   ├── slice_character_sheets.py  # 元 tomari-guruguru を MIT 継承 (Phase 1.12)
│   ├── requirements.txt     # Python 依存
│   └── LICENSE-third-party  # 依存ライセンス一覧
├── docs/                    # 設計ドキュメント
│   ├── PLAN.md              # 全体設計 (v0.4.8)
│   ├── PHASE1.md            # Phase 1 詳細設計
│   ├── PHASE2.md            # Phase 2 詳細設計 (MediaPipe 確定)
│   ├── PHASE3.md            # Phase 3 詳細設計
│   └── 新キャラ差し替え手順.md  # 元 MIT 翻訳、Phase 1.12 で port 内容に更新
├── scripts/                 # ビルドスクリプト
│   ├── build.ps1 / build.sh
│   └── dev.ps1 / dev.sh
├── .gitignore
├── LICENSE                  # MIT
├── README.md
├── go.mod / go.sum
```

## ロードマップ

| Phase | 状態 | 内容 |
|---|---|---|
| **Phase 1.1 〜 1.12** | ✅ **完了** | MVP: 透過 + クリックスルー + アトラス + マウス追従 (Y軸反転なし) + まばたき + メインマイク口パク + Tweaks + CJK フォント + ビルドスクリプト + slice ツール (Phase 1.12 で元 648 行版 MIT 継承) |
| **Phase 1.13b** | ✅ 完了 | UI 非表示ショートカット (`Ctrl+Shift+H` で Tweaks + 設定 UI を**全部トグル**表示/非表示。OBS ウィンドウキャプチャで UI が映り込まないようにする) |
| **Phase 1.13a** | ✅ 完了 | マイク選択 + TOML 永続化 — malgo `Devices` 列挙 → ebitenui `ListComboButton` (ComboBox) ドロップダウン → 選択デバイスの malgo 内部 ID を `os.UserConfigDir()/GoTuber/config.toml` に保存 → 再起動時復元 (ID 照合で重複表示名も問題なし) |
| **Phase 1.14** | ✅ 完了 | **終了ショートカット削除 + audio lifecycle fix** — `Esc` / `Q` キー検出と `killswitch.Install()` の Windows 限定削除 (Unix は `signal.Notify` 維持 = Ctrl+C graceful)。**真因判明**: Phase 1.13a visual test で F1 押下時に ListComboBox 初期選択 → `onDeviceSelected("")` → `Mover.Restart("")` → `NewCaptureByID()` の defer で成功 path も context 解放 → 次回 `Capture.Stop()` で double-free → 即終了。修正: `cleanupCtx` フラグで success/error 分離、`Mover.Restart` を失敗時旧 capture 温存化、main.go の guard で同一 device ID 選択 no-op。 |
| Phase 2 | ✅ 完了 | カメラ VTuber: 頭の方向 + 瞬き (EAR)、Python サイドカー + localhost TCP JSONL |
| Phase 3 | Creator Tools: 1 枚入力 → A 25 枚 → 目眉/口マスク → AI 補完で 150 枚。Phase 3.6 で Depth Anything v3 を使った Morph Renderer 用 depth map のオフライン生成を実装済み（RTX 2060 で A〜F 全150枚 GPU 生成完了） |
| Phase 4 | 計画中 | Morph Renderer: αブレンド + mesh + depth-weighted elastic morph でセル切り替えを滑らかに見せる |

設計判断とフェーズ詳細: [docs/PLAN.md](docs/PLAN.md) / [docs/PHASE1.md](docs/PHASE1.md) 参照。

Phase 3.6 の depth map 生成環境は `tools/setup-depth.ps1` / `tools/setup-depth.sh` でセットアップする。

## キーボードショートカット

| ショートカット | 機能 | Phase |
|---|---|---|
| `F1` | Tweaks パネル表示/非表示 | 1.8 |
| `Ctrl+Shift+H` | 全ての UI (Tweaks + 設定) を**一括トグル**表示/非表示。**配信時に使用** (OBS ウィンドウキャプチャで UI が映らない) | 1.13b (予定) |
| **ウィンドウ X ボタン** | 終了 (推奨) | 1.1 |
| ~~`Esc` / `Q`~~ | ~~終了 (kill switch)~~ | **削除済み (Phase 1.14)**: F1/Esc 即終了バグの真因は audio lifecycle の double-free。代替: Tweaks Quit ボタン or ウィンドウ X ボタン |
| `Ctrl+C` | 終了 (コンソールから) | Unix: graceful (`signal.Notify` 経由) / Windows: Go runtime デフォルト即終了 |

## テスト

```bash
go test ./...
```

Phase 1.10 時点で:
- `internal/killswitch` — 1 テスト（signal handler 動作）
- `internal/mouse` — 5 テスト（クランプ / スムージング / セルマッピング / responsiveness 0 / 範囲外クランプ）
- `internal/blink` — 6 テスト（分布 / 間隔範囲 / 間隔分布 / 瞬き継続時間 / 状態遷移 / seed）
- `internal/audio` — 6 テスト（AttackRelease / Hysteresis / OpenToClosed / RMS / DecodePCM / Mover）
- `internal/tweaks` — 4 テスト（State defaults / Quit / PanelVisible / LoadFontFace）

## ライセンス

- **プログラムコード**: MIT License（[LICENSE](LICENSE)）— GoTuber 本体コードは自前実装
- **埋め込みフォント (Gen Interface JP)**: SIL Open Font License 1.1
- **依存ライブラリ**: 各 OSS ライセンス（[tools/LICENSE-third-party](tools/LICENSE-third-party)）
- **発想元 / 参考元**: `tomari-guruguru`（アーキテクチャ検討・制作フローの参考）
- **継承ツール**: `tools/slice_character_sheets.py` は `tomari-guruguru` 由来の MIT ツール
- **GoTuber 同梱 `_default` サンプルキャラクター**: 動作検証・テスト利用は自由。動画/配信利用は yosia の明示許可が必要。再配布禁止（詳細は `docs/PHASE3.md` 3.0.8）

`tomari-guruguru` は前身 / 参考元だが、GoTuber の本体コードは自前実装。

## クレジット

- 前身: [tomari-guruguru](https://github.com/rotejin/tomari-guruguru) by rotejin
- スタック:
  - [Ebitengine v2.9.9](https://github.com/hajimehoshi/ebiten) — 描画エンジン
  - [malgo v0.11.25](https://github.com/gen2brain/malgo) — オーディオ（miniaudio バインディング）
  - [ebitenui v0.7.3](https://github.com/ebitenui/ebitenui) — UI ウィジェット
  - [golang.org/x/image v0.42.0](https://pkg.go.dev/golang.org/x/image) — WebP デコーダー
  - [Gen Interface JP v0.6.2](https://github.com/yamatoiizuka/gen-interface-jp) — UI フォント (Inter + Noto Sans JP 合成)

## ステータス

✅ **Phase 1 コア完了 (1.1〜1.12)** — コードレビュー対応済み、`go test ./...` 全パス (Windows バイナリ 19.5 MB / Linux バイナリ 25 MB)。キャラクターシステムは元 [tomari-guruguru](https://github.com/rotejin/tomari-guruguru) から 100% port (camelCase 設定、Y軸反転なし、1200×1200 anchored WebP、元 648 行スライスツール MIT 継承)。

🔜 **次の予定**: Phase 3 Creator Tools（Phase 3.1 build-a CLI → Phase 3.2 マスク生成）。その後 Phase 4 Morph Renderer（`docs/PHASE4.md`）へ進む。Phase 3.6 Depth Map Generator は完了済み（A〜F 全150枚、DA3 GPU 生成済み）。

- プラン: [docs/PLAN.md](docs/PLAN.md) v0.4.8
- Phase 1.12 詳細: [docs/PHASE1.md](docs/PHASE1.md) Section 9
- Phase 1.13 (1.13a/1.13b) 詳細: [docs/PHASE1.md](docs/PHASE1.md) Section 10
- Phase 1.14 詳細: [docs/PHASE1.md](docs/PHASE1.md) Section 11
- Phase 2 詳細: [docs/PHASE2.md](docs/PHASE2.md)
- Phase 3 詳細: [docs/PHASE3.md](docs/PHASE3.md)
- Phase 4 詳細: [docs/PHASE4.md](docs/PHASE4.md)
- 設計判断: pure Go 書き直し採用（Wails / headless JS 比較の上、Section 0.5 参照）
- ビルド方針: Windows 10/11（mingw-w64 クロスコンパイル）または WSL Ubuntu（gcc）
- **視覚テストはユーザー側で実施予定**（実装完了 → 実行確認は手動）
- Phase 1 スコープ: マウス追従 + メインマイク Realtime 口パク + 透過 + まばたき + Tweaks + CJK フォント + 設定永続化（音声ファイル口パクは post_release、Phase 2 で MediaPipe 即採用確定）
