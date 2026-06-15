# GoTuber

> **軽量 PNG アバターアプリ** — Windows 優先、Golang 製、OBS 透過キャプチャ対応

[![Status](https://img.shields.io/badge/status-Phase%201%20WIP-yellow)](#roadmap)
[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go)](#ビルド)
[![License: MIT](https://img.shields.io/badge/code-MIT-blue.svg)](LICENSE)

**15MB の exe 1 個で動く透過アバターアプリ**。PNG 画像を 25 方向に動かしてアバター表示 + **メインマイクで同時 Realtime 口パク**。OBS のウィンドウキャプチャに重ねて配信やビデオ通話に使う。

前身: [tomari-guruguru](https://github.com/rotejin/tomari-guruguru) を **Golang で完全書き直し**（React/Vite 製 → 単一バイナリ）。

## 特徴

### Phase 1（MVP）
- **PNG アバター表示** — 5×5 角度 × 6 表情 = 150 枚のスプライト切替
- **マウス追従** — カーソルに 25 方向で振り向く
- **メインマイクで同時 Realtime 口パク** — 喋ると口の 3 段階（とじ/はんびらき/ぜんかい）が自動切替。VTuber 配信用途の主機能
- **自動まばたき** — 不規則なタイミング（自然な分布、二度・ゆっくり含む）
- **透過背景ウィンドウ** — OBS でウィンドウキャプチャするだけ（クロマキー不要）
- **クリックスルー** — キャラの背後をクリックできる（配下アプリの操作を邪魔しない）
- **Tweaks パネル** — 追従速度・口パク感度・キャラサイズ・背景色を画面右下の Tweaks から調整
- **Kill Switch** — `Esc` キー / `Ctrl+C` で即終了

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
| **GoTuber** | **15MB** | **exe 単体・透過キャプチャ・Realtime マイク口パク** | **超軽量 + 配信者向け** |

**「Live2D じゃなくていい、PNG の 5×5 アバターで十分、かつ 1 バイナリで完結したい」人向け**。

## 配信者の使い方

```
1. GoTuber.exe をダウンロード (15MB)
2. exe ダブルクリックで起動
3. 自分のキャラ PNG を assets/characters/<name>/ に配置
   （or デフォルトのプレースホルダで起動）
4. OBS を起動
5. ソース追加 → ウィンドウキャプチャ → GoTuber を選択
6. 配信開始
7. 喋る → キャラの口が動く
8. (Phase 2) カメラの前に座る → 顔でキャラが動く
9. 終了したい時 → Esc キー
```

## 動作環境

| OS | 対応 | 備考 |
|---|---|---|
| **Windows 10 / 11** | ✅ 優先 | MSVC ビルド / DirectX |
| **Linux (X11)** | ✅ 検証済 | KASOU (Debian 13) で実機テスト |
| **Linux (Wayland)** | ⚠️ 要検証 | Ebitengine Wayland サポートに依存 |
| **macOS 11+** | ✅ 対応予定 | Metal |

- WebView2 不要（pure ネイティブ描画）
- マイク・カメラは対応 OS の標準 API でアクセス

## 開発者向け

### 必要環境

- **Go 1.25+**（Ebitengine v2.9 + `golang.org/x/image v0.42` 要件）
- C コンパイラ（MSVC または gcc）— CGo (malgo) のため
- Phase 2 以降: OpenCV 4.13.0（gocv v0.43.0 要件）
- Python 3 + Pillow, numpy（スライス生成時のみ）

### ビルド

```bash
# WSL Ubuntu（KASOU 用 Linux バイナリ）
cd /path/to/GoTuber
CGO_ENABLED=1 go build -ldflags "-s -w" -o bin/gotuber ./cmd/gotuber

# Windows native（PowerShell）
$env:CGO_ENABLED = "1"
go build -ldflags "-s -w" -o bin\gotuber.exe .\cmd\gotuber

# テスト
go test ./... -v -race
```

### 開発ループ

```bash
go run ./cmd/gotuber --character assets/characters/_default
```

## プロジェクト構成

```
GoTuber/
├── cmd/gotuber/             # エントリポイント
├── internal/                # 内部パッケージ
│   ├── app/                 # ebiten.Game 実装
│   ├── audio/               # malgo 完結 (mic + ファイル + スピーカー)
│   ├── character/           # アトラス + 設定
│   ├── camera/              # Phase 2 (gocv)
│   ├── avatar/              # 描画ロジック
│   ├── mouse/               # マウス追従
│   ├── blink/               # 自動まばたき
│   ├── tweaks/              # Tweaks パネル
│   └── killswitch/          # SIGINT + Esc
├── assets/
│   ├── characters/          # デフォルトキャラ + ユーザー配置先
│   └── fonts/               # CJK フォント埋め込み
├── config/                  # YAML 設定
├── tools/                   # Python スライスツール + 依存ライブラリ
├── docs/                    # 設計ドキュメント
│   ├── PLAN.md              # 全体設計 (v0.4.3)
│   ├── PHASE1.md            # Phase 1 詳細設計
│   ├── PHASE2.md            # Phase 2 詳細設計 (保留中)
│   └── PHASE3.md            # Phase 3 詳細設計
├── scripts/                 # ビルド・デプロイスクリプト
├── .gitignore
├── LICENSE                  # MIT
└── README.md
```

## ロードマップ

| Phase | 状態 | 内容 |
|---|---|---|
| **Phase 1** | 着手準備完了 | MVP: マウス追従 + メインマイク Realtime 口パク + 透過キャプチャ + kill switch |
| Phase 2 | **保留中** | カメラ VTuber: 顔追従 + 口の自動検出（Q8 で再評価待ち） |
| Phase 3 | 未着手 | VMC Protocol 出力 |

設計判断とフェーズ詳細: [docs/PLAN.md](docs/PLAN.md) 参照。

## ライセンス

- **プログラムコード**: MIT License（[LICENSE](LICENSE)）
- **キャラクター画像・音声**: **非商用・再配布禁止**（前身プロジェクト `tomari-guruguru` と同じ制約）

`tools/slice_character_sheets.py`（Python スライスツール）は `tomari-guruguru` から MIT で継承。

## クレジット

- 前身: [tomari-guruguru](https://github.com/rotejin/tomari-guruguru) by rotejin
- スタック:
  - [Ebitengine v2.9.9](https://github.com/hajimehoshi/ebiten) — 描画エンジン
  - [malgo v0.11.25](https://github.com/gen2brain/malgo) — オーディオ（miniaudio バインディング）
  - [ebitenui v0.7.3](https://github.com/ebitenui/ebitenui) — UI ウィジェット
  - [golang.org/x/image v0.42.0](https://pkg.go.dev/golang.org/x/image) — WebP デコーダー

## ステータス

⚠️ **開発初期段階** — Phase 1.1（プロジェクト雛形 + 最小 main.go + kill switch）着手前。

- プラン: [docs/PLAN.md](docs/PLAN.md) v0.4.3（Q1/Q3/Q4/Q12 確定、Q6/Q8 保留反映済み）
- 設計判断: pure Go 書き直し採用（Wails / headless JS 比較の上、Section 0.5 参照）
- ビルド方針: Windows 10/11（PowerShell + MSVC）または WSL Ubuntu（gcc）。KASOU（Debian MiniPC）は **ランタイム専用**
- Phase 1 スコープ: マウス追従 + メインマイク Realtime 口パク + 透過 + kill switch（音声ファイル口パク・カメラは Phase 1.5+ / Phase 2+ で再評価）
