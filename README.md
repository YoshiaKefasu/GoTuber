# GoTuber

> **軽量 PNG アバターアプリ** — Windows 優先、Golang 製、OBS 透過キャプチャ対応

[![Status](https://img.shields.io/badge/status-Phase%201%20Done-brightgreen)](#roadmap)
[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go)](#ビルド)
[![License: MIT](https://img.shields.io/badge/code-MIT-blue.svg)](LICENSE)

**19.5 MB の exe 1 個で動く透過アバターアプリ**。PNG 画像を 25 方向に動かしてアバター表示 + **メインマイクで同時 Realtime 口パク**。OBS のウィンドウキャプチャに重ねて配信やビデオ通話に使う。

前身: [tomari-guruguru](https://github.com/rotejin/tomari-guruguru) を **Golang で完全書き直し**（React/Vite 製 → 単一バイナリ）。

## 特徴

### Phase 1（MVP・実装完了）
- **PNG アバター表示** — 5×5 角度 × 6 表情 = 150 枚のスプライト切替
- **マウス追従** — カーソルに 25 方向で振り向く
- **メインマイクで同時 Realtime 口パク** — 喋ると口の 3 段階（とじ/はんびらき/ぜんかい）が自動切替。VTuber 配信用途の主機能
- **自動まばたき** — 不規則なタイミング（自然な分布、二度・ゆっくり含む）
- **透過背景ウィンドウ** — OBS でウィンドウキャプチャするだけ（クロマキー不要）
- **クリックスルー** — キャラの背後をクリックできる（配下アプリの操作を邪魔しない）
- **Tweaks パネル** — 追従速度・自動まばたき ON/OFF・口パク ON/OFF を `F1` キーまたは Quit ボタンで操作
- **CJK フォント埋め込み** — Gen Interface JP Regular (Inter + Noto Sans JP) をバイナリ同梱
- **Kill Switch** — `Esc` キー / `Q` キー / `Ctrl+C` / Tweaks の Quit ボタン で即終了

> **Phase 1 スコープ外**（Phase 1.5+ で再評価）: 音声ファイル口パク（mp3/wav/ogg）

### Phase 2（カメラ VTuber・保留中）
- **Webcam 顔追従** — 顔の動き（左右・上下・傾き）にキャラが追従
- **口の自動検出** — カメラ映像の口の動きで口パク
- **自動まばたき（カメラ）** — 目の瞬きでキャラがまたたき
- **フェイルセーフ** — 顔が消えたら自動的にマイク/マウスモードに切替

### Phase 3（VTuber ソフト連携・未着手）
- **VMC Protocol 出力** — VTube Studio / VSeeFace / EVMC4U 等に UDP で送信
- GoTuber のマイク/カメラ情報をブレンドシェイプ（A, I, U, E, O, Blink）として他ソフトのキャラに反映

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
10. 終了したい時 → Esc キー / Ctrl+C
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
- Phase 2 以降: OpenCV 4.13.0（gocv v0.43.0 要件）
- Python 3 + Pillow, numpy（スライス生成時のみ）— `pip install -r tools/requirements.txt`

### ビルド

#### Windows native
```powershell
.\scripts\build.ps1            # リリースビルド (19.5 MB)
.\scripts\build.ps1 -Dev       # デバッグビルド
.\scripts\build.ps1 -Clean     # bin/ 削除してビルド
```

#### WSL Ubuntu / Linux
```bash
./scripts/build.sh             # リリースビルド
./scripts/build.sh --dev       # デバッグビルド
./scripts/build.sh --skip-test # テストスキップ
```

#### 開発ループ（ビルド + 実行）
```powershell
.\scripts\dev.ps1              # Windows
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

各シートの画像フォーマット: `r{0-4}c{0-4}.png`（5行 × 5列 = 25枚）

### 自分のキャラを追加

1. 5×5×6 = 150 枚の PNG を用意
2. ディレクトリ: `assets/characters/<your-character>/{A-F}/r{0-4}c{0-4}.png`
3. `config/default.yaml` の `base_path` を変更
4. または `config/<your-character>.yaml` を作成

スライス生成は `tools/slice_character_sheets.py` (Phase 1.11 実装、MIT 継承)。

```bash
# インストール
pip install -r tools/requirements.txt

# 単一シート分割
python tools/slice_character_sheets.py \
    --input sheet_A.png --output assets/characters/my_char/A

# 6 シート一括
python tools/slice_character_sheets.py \
    --input-sheet "A:sheet_A.png,B:sheet_B.png,C:sheet_C.png,D:sheet_D.png,E:sheet_E.png,F:sheet_F.png" \
    --output-dir assets/characters/my_char
```

**テスト実行**:
```bash
# pytest 不要なフォールバック
python tools/slice_character_sheets_test.py

# pytest 使用
python -m pytest tools/slice_character_sheets_test.py -v
```

### キー操作

| キー | 動作 |
|---|---|
| `F1` | Tweaks パネル表示/非表示 |
| `Esc` | 終了 |
| `Q` | 終了 |
| `Ctrl+C` | 終了（コンソールから） |

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
│   ├── character/           # アトラス + 設定 + バリデーション
│   ├── mouse/               # マウス追従 (5×5 グリッド)
│   ├── blink/               # 自動まばたき (22/6/72 分布)
│   ├── tweaks/              # Tweaks パネル (ebitenui + CJK フォント)
│   │   ├── font.go          # //go:embed で TTF 埋め込み
│   │   ├── panel.go         # ebitenui UI
│   │   ├── state.go         # パネル状態
│   │   └── assets/fonts/    # Gen Interface JP Regular.ttf
│   ├── killswitch/          # SIGINT + Esc
│   └── character/           # アトラス + 設定
├── assets/
│   ├── characters/          # デフォルトキャラ + ユーザー配置先
│   │   └── _default/        # 6 シート × 25 枚 = 150 プレースホルダ
│   └── (Tweaks フォントは internal/tweaks/assets/ 配下)
├── config/
│   └── default.yaml         # デフォルトキャラ設定
├── tools/
│   ├── genplaceholder/      # プレースホルダ生成 (Phase 1.3)
│   ├── slice_character_sheets.py  # 5×5 スプライトシート分割 (Phase 1.11)
│   ├── requirements.txt     # Python 依存
│   └── LICENSE-third-party  # 依存ライセンス一覧
├── docs/                    # 設計ドキュメント
│   ├── PLAN.md              # 全体設計 (v0.4.3)
│   ├── PHASE1.md            # Phase 1 詳細設計
│   ├── PHASE2.md            # Phase 2 詳細設計 (保留中)
│   └── PHASE3.md            # Phase 3 詳細設計
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
| **Phase 1** | ✅ **完了** | MVP: 透過 + クリックスルー + アトラス + マウス追従 + まばたき + メインマイク口パク + Tweaks + CJK フォント + ビルドスクリプト + slice ツール (Phase 1.11) |
| Phase 2 | **保留中** | カメラ VTuber: 顔追従 + 口の自動検出（Q8 で再評価待ち） |
| Phase 3 | 未着手 | VMC Protocol 出力 |

設計判断とフェーズ詳細: [docs/PLAN.md](docs/PLAN.md) / [docs/PHASE1.md](docs/PHASE1.md) 参照。

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

- **プログラムコード**: MIT License（[LICENSE](LICENSE)）
- **埋め込みフォント (Gen Interface JP)**: SIL Open Font License 1.1
- **依存ライブラリ**: 各 OSS ライセンス（[tools/LICENSE-third-party](tools/LICENSE-third-party)）
- **キャラクター画像・音声**: **非商用・再配布禁止**（前身プロジェクト `tomari-guruguru` と同じ制約）

`tools/slice_character_sheets.py`（Python スライスツール）は `tomari-guruguru` から MIT で継承。

## クレジット

- 前身: [tomari-guruguru](https://github.com/rotejin/tomari-guruguru) by rotejin
- スタック:
  - [Ebitengine v2.9.9](https://github.com/hajimehoshi/ebiten) — 描画エンジン
  - [malgo v0.11.25](https://github.com/gen2brain/malgo) — オーディオ（miniaudio バインディング）
  - [ebitenui v0.7.3](https://github.com/ebitenui/ebitenui) — UI ウィジェット
  - [golang.org/x/image v0.42.0](https://pkg.go.dev/golang.org/x/image) — WebP デコーダー
  - [Gen Interface JP v0.6.2](https://github.com/yamatoiizuka/gen-interface-jp) — UI フォント (Inter + Noto Sans JP 合成)

## ステータス

✅ **Phase 1 完了** — 全 Phase 1.1 〜 1.9 実装 + コードレビュー対応済み、`go test ./...` 全パス、Windows バイナリ 19.5 MB / Linux バイナリ 25 MB。

- プラン: [docs/PLAN.md](docs/PLAN.md) v0.4.3
- 設計判断: pure Go 書き直し採用（Wails / headless JS 比較の上、Section 0.5 参照）
- ビルド方針: Windows 10/11（mingw-w64 クロスコンパイル）または WSL Ubuntu（gcc）
- **視覚テストはユーザー側で実施予定**（実装完了 → 実行確認は手動）
- Phase 1 スコープ: マウス追従 + メインマイク Realtime 口パク + 透過 + まばたき + Tweaks + CJK フォント（音声ファイル口パク・カメラは Phase 1.5+ / Phase 2+ で再評価）
