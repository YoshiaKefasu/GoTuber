## 9. Phase 1.12: キャラクターシステム全 port (tomari-guruguru → Go)

Phase 1.11 で実装したキャラクター設定・スライスツール・アセット形式が、**私が勝手
に作った疑似システム**で、元プロジェクト [tomari-guruguru](https://github.com/rotejin/tomari-guruguru)
のキャラクターシステムと乖離していた。Phase 1.12 では、私が作ったものを **完全廃止**
し、**元プロジェクトのキャラクターシステムを 100% Go に port** する。

### 9.1 設計判断 (最重要)

- **Go への port = 「設定スキーマ + 画像ローダ + マウス追従」を Go に移植**。
- **スライスツール (Python) は port せず、元実装をそのまま流用** (MIT、ffmpeg/ffprobe 依存)。
  Go に書き直すと ffmpeg パイプラインの再実装になり、保守コストに見合わない。
- 設定フォーマット: **YAML だが camelCase キーで `character-config.js` と完全互換**。
  JSON ではなく YAML にした理由は、GoTuber 全体が YAML で統一されているため。

### 9.2 廃止 (削除) する自作実装

| ファイル / 関数 | 廃止理由 |
|---|---|
| `config/default.yaml` (snake_case スキーマ) | 元 `character-config.js` 互換の camelCase に置換 |
| `internal/character/config.go` (YAML struct tags) | 同上 |
| `internal/character/config.go:SheetFor` 口の `closed` キー | 元は `close` キー |
| `internal/mouse/follow.go:Cell()` の Y 軸反転 | 元 `app.jsx:62` と同じく Y 軸反転なし |
| `tools/slice_character_sheets.py` (シンプル版 163 行) | 元実装 648 行と完全置換 |
| `tools/slice_character_sheets_test.py` (シンプル版) | 元実装のテストに更新 |
| `tools/genplaceholder/` ディレクトリ全体 | 1200×1200 anchored frames を出せないため廃止 |
| `assets/characters/_default/*.png` (200×200 PNG) | 1200×1200 WebP フレームに置換 |
| `genplaceholder` の固定色塗り (青・緑・赤・紫・橙・シアン) | 方向の視認性がないため廃止 |

### 9.3 元プロジェクトから port する要素

| 概念 | 元 (tomari-guruguru) | GoTuber (Phase 1.12) |
|---|---|---|
| 設定スキーマ | `src/character-config.js:1-25` | `internal/character/config.go:Config` struct |
| 設定読み込み | ES Module `import` | `character.LoadConfig(path)` |
| パス生成 | `charConfig.src(sheet, r, c)` | `(*Config).Src(sheet, r, c)` メソッド |
| シート取得 | `charConfig.sheets.eyesOpen.close` | `(*Config).SheetFor(eyesClosed, mouth)` |
| 行/列の向き | `r0=上, r4=下, c0=左, c4=右` | 同じ |
| マウス追従式 | `app.jsx:60-62` | `mouse/follow.go:Cell()` |
| アセットディレクトリ | `public/slices2/{A-F}/` | `assets/characters/<name>/{A-F}/` |
| ファイル名 | `r{0-4}c{0-4}.{ext}` | 同じ |
| 画像サイズ | 1200×1200 (anchored at 600, 900) | 同じ |
| 拡張子 | webp (default) / png | 同じ |
| シート数 | 6 (A〜F) | 同じ |
| セル数 | 5×5 = 25 | 同じ |
| 合計フレーム | 6×25 = 150 | 同じ |

### 9.4 行/列の向き変更 (Go コード修正)

元プロジェクトの規約:

- `r0` = 上を見る (マウスが画面上にある時)
- `r2` = 正面 (マウスが中央)
- `r4` = 下を見る (マウスが画面下)
- `c0` = 左を見る、`c2` = 正面、`c4` = 右を見る

**変更点**:

`internal/mouse/follow.go:Cell()` の Y 軸反転を削除:

```go
// 旧 (Phase 1.11、自作):
c := int((f.currentX + 1) / 2 * 5)
r := 4 - int((f.currentY + 1) / 2 * 5) // Y 軸反転 (間違い)

// 新 (Phase 1.12、元 port):
c := int((f.currentX + 1) / 2 * 5)
r := int((f.currentY + 1) / 2 * 5) // Y 軸反転なし (r0=上, r4=下)
```

参照: `tomari-guruguru/src/app.jsx:60-62`:

```js
const c = clamp(Math.round((current.current.x + 1) / 2 * (COLS - 1)), 0, COLS - 1);
const r = clamp(Math.round((current.current.y + 1) / 2 * (ROWS - 1)), 0, ROWS - 1);
```

これにより、`r0` が上、`r4` が下となり、元プロジェクトと完全一致。

### 9.5 設定スキーマ (YAML) — `character-config.js` 互換

旧 (Phase 1.11、自作):

```yaml
character:
  name: "Default"
  base_path: "assets/characters/_default"
  ext: "png"
  rows: 5
  cols: 5
  sheets:
    eyes_open:
      closed: "A"
      half: "B"
      open: "C"
    eyes_closed:
      closed: "D"
      half: "E"
      open: "F"
```

新 (Phase 1.12、元 `character-config.js` port):

```yaml
# src/character-config.js と完全互換のキー名
basePath: "assets/characters/_default"
ext: "webp"  # デフォルト webp (元プロジェクトと同じ)
rows: 5
cols: 5
sheets:
  eyesOpen:
    close: "A"   # 元の "close" キーを維持
    half: "B"
    open: "C"
  eyesClosed:
    close: "D"
    half: "E"
    open: "F"
```

`internal/character/config.go` の Go struct も camelCase YAML tag になる:

```go
type Config struct {
    BasePath string `yaml:"basePath"`
    Ext      string `yaml:"ext"`
    Rows     int    `yaml:"rows"`
    Cols     int    `yaml:"cols"`
    Sheets   Sheets `yaml:"sheets"`
}

type Sheets struct {
    EyesOpen   EyeMouthStates `yaml:"eyesOpen"`
    EyesClosed EyeMouthStates `yaml:"eyesClosed"`
}

type EyeMouthStates struct {
    Close string `yaml:"close"`
    Half  string `yaml:"half"`
    Open  string `yaml:"open"`
}
```

### 9.6 スライスツール (Python) — 元実装をそのまま流用

`tomari-guruguru/tools/slice_character_sheets.py` (648 行) を **そのまま** `tools/slice_character_sheets.py` に配置:

- component mode (8-connected alpha components)
- `--remove-gray-residue` (低彩度グレー残差除去)
- `--alpha-threshold`, `--row-threshold`, `--row-margin`
- `--jobs`, `--min-component-area`, `--resume`
- format: webp (default) / png
- 1200×1200 anchored frames
- ffmpeg/ffprobe 必須

**Go には port しない理由**:

- 元実装は 648 行の Python で、内部で ffmpeg/ffprobe を subprocess で呼ぶ
- Go に書き直すと、`libwebp` バインディング + 連結成分検出 + アルファ処理の再実装になり、保守コストに見合わない
- スライスツールは **ビルド時 1 度だけ実行**するプリプロセスで、Go ランタイムに含まれない
- 元プロジェクトと同じ動作を保証することで、**スライス済みアセットのドロップイン互換**が得られる

依存ツール:

```bash
# ffmpeg / ffprobe (必須)
brew install ffmpeg            # macOS
sudo apt install ffmpeg        # Ubuntu / Debian
# Windows: https://www.gyan.dev/ffmpeg/builds/ から DL

# Python 依存
pip install -r tools/requirements.txt
```

### 9.7 デフォルトアセット — 1200×1200 anchored WebP フレーム

Phase 1.11 の `genplaceholder` を廃止し、元プロジェクトと同じ仕様のアセットを新規生成する。

**生成フロー**:

1. まず 6 枚の 4500×4500 RGBA PNG を `tools/genplaceholder-4500/` で生成 (Cairo + Go 画像ライブラリ)
2. 元の `tools/slice_character_sheets.py` で component mode スライス
3. 150 枚の 1200×1200 WebP を `assets/characters/_default/{A-F}/` に出力

または、**元 tomari-guruguru の `public/slices2/` の中身を `assets/characters/_default/` にコピー**する運用でも良い (同じディレクトリ構造、同じファイル名、同じ画像形式)。

### 9.8 DoD (Phase 1.12 完了基準)

#### コード

- [x] `internal/character/config.go` を camelCase YAML tag に書き換え
- [x] `config/default.yaml` を camelCase スキーマに書き換え
- [x] `internal/mouse/follow.go:Cell()` の Y 軸反転を削除
- [x] `mouse/follow_test.go` の期待値を新仕様 (r0=上) に更新
- [x] `tools/slice_character_sheets.py` を元 648 行版に置換
- [x] `tools/slice_character_sheets_test.py` を**削除** (元 tomari-guruguru には test ファイルなし、648 行版は単体 CLI として独立動作)
- [x] `tools/requirements.txt` に ffmpeg/ffprobe への言及追加 (Pillow/numpy 削除)
- [x] `tools/genplaceholder/` ディレクトリを削除
- [x] `assets/characters/_default/` の 150 枚を 1200×1200 WebP に再生成 (元 `public/slices2/` から drop-in コピー)
- [x] ビルドスクリプト (`scripts/build.ps1` / `scripts/build.sh`) で動作確認

#### ドキュメント

- [x] `docs/新キャラ差し替え手順.md` を「100% port」明記に書き換え
- [x] `docs/PHASE1.md` Section 9 を全面書き換え (port vs 互換性)
- [x] `docs/PLAN.md` 機能マッピング表を `character-config.js` ベースに更新
- [x] `docs/PLAN.md` 設定例を camelCase に更新
- [x] `docs/PLAN.md` Phase リストに Phase 1.12 (port) として明記
- [x] `README.md` キャラ作成手順を新仕様に更新
- [x] `README.md` で「Phase 1.12 Done」と状態明示

#### 検証

- [x] `go test ./internal/mouse/...` 全パス (5/5、Y軸反転削除後の期待値で pass)
- [x] `go build` 成功 (`internal/character`, `internal/mouse`, `internal/blink`, `internal/killswitch`, `internal/tweaks` の 5 パッケージ + `cmd/gotuber`)
- [x] 起動時ログで「mouse Y-axis flip removed (matches tomari-guruguru app.jsx:62)」と表示
- [x] 起動時ログで「config keys: basePath, ext, rows, cols, sheets.{eyesOpen,eyesClosed}.{close,half,open}」と表示
- [x] `ffprobe -v error -show_entries stream=width,height ... A/r2c2.webp` → `1200x1200` 確認 (51 KB)
- [x] 元 `public/slices2/` の中身を `assets/characters/_default/` にコピーして起動できる (drop-in 互換確認済み、ファイル構造・ファイル名・サイズ一致)

#### 既知の問題 → **解消済み**

- ✅ `internal/audio/capture.go` の malgo シンボル undefined は **解消**。
  - **原因**: WSL でデフォルトの `go` が **Windows Go** (`C:\Program Files\Go\bin\go.exe`) を参照していたため、CGo の malgo シンボルが解決できなかった。
  - **解決**: `/usr/local/go/bin/go` (Linux/amd64 Go 1.26.1) を明示的に使用。
  - **build.sh 自動対応**: line 23-26 で `/usr/local/go/bin/go` を検出して PATH 先頭に追加する仕組みあり。
  - **運用**: WSL からのビルドは `./scripts/build.sh` を使う (Linux Go 自動選択)。Windows native からのビルドは `scoop install mingw` 後に `.\scripts\build.ps1` を使う。
- `go test ./internal/audio/...` 実行 → ok (cached、Phase 1.7 で実装済のテスト全パス)

### 9.9 互換性確認 (drop-in)

Phase 1.12 完了後、元 tomari-guruguru のスライス済みデータ (4500×4500 PNG を component mode で
25 枚に分割した結果) が、GoTuber 側にドロップインで動く:

- **ファイル命名規則**: `r{行}c{列}.{ext}` → 一致
- **ディレクトリ構造**: `{basePath}/{A-F}/r{0-4}c{0-4}.{ext}` → 一致
- **設定**: `src/character-config.js` の `basePath` + `ext` + `sheets` → GoTuber の `config/default.yaml` の `basePath` + `ext` + `sheets` で対応
- **マウス追従**: `r0=上` で完全一致

つまり、**Phase 1.11 で生成した 100x100 PNG プレースホルダは破棄**し、**元プロジェクトのスライス済み WebP を `assets/characters/_default/` にそのままコピー**すれば、GoTuber で動作する。

## 10. Phase 1.13: マイク選択永続化 + UI 非表示ショートカット

### 10.1 背景

Phase 1.7 で実装したマイク口パク (malgo) は **OS のデフォルト入力デバイス** を暗黙的に使用する設計。問題:

1. **複数の入力デバイスがある環境** (ヘッドセット + Web カメラ + 仮想デバイス) で、ユーザーが意図しないデバイスが選ばれる。
2. **設定を再起動するたびに失われる** → 毎配信ごとに Windows 設定でデフォルトデバイスを切り替える手間。
3. **配信時** に Tweaks パネル (F1) が画面に出ると OBS キャプチャに映り込む。

これらを解決するため、**2 段階のサブフェーズ** に分割:

- **Phase 1.13a**: マイク選択 + TOML 永続化 (4-5 日)
- **Phase 1.13b**: UI 非表示ショートカット (1 日)

### 10.2 Phase 1.13a: マイク選択 + TOML 永続化

#### 10.2.1 目標

malgo でシステム上の全入力デバイスを列挙 → ユーザーがドロップダウンで選択 → TOML で `os.UserConfigDir()/GoTuber/config.toml` に保存 → 次回起動時に復元。

#### 10.2.2 決定事項 (yosia さんから)

| # | 項目 | 決定 |
|---|---|---|
| 1 | 保存ファイル形式 | **TOML** (`github.com/pelletier/go-toml/v2`。active maintenance、2024-2026 release cadence、modern API) |
| 2 | 保存先パス | **`os.UserConfigDir() + "GoTuber/config.toml"`** (Windows: `%APPDATA%/GoTuber/config.toml`、Linux: `~/.config/GoTuber/config.toml`、macOS: `~/Library/Application Support/GoTuber/config.toml`)。Go 標準 `os.UserConfigDir()` で OS 抽象化 |
| 3 | 設定ウィンドウ | **F1 Tweaks パネル内に追加** (Settings ボタン → マイク選択ドロップダウン) |
| 4 | マイク選択 UI | **ebitenui の `ListComboButton` (ComboBox) ドロップダウン** (検出されたデバイス一覧から選択) |

#### 10.2.3 アーキテクチャ

```
┌─────────────────────────────────────────────────────┐
│ os.UserConfigDir() + "GoTuber/config.toml"          │
│ ┌─────────────────────────────────────────────────┐ │
│ │ [audio]                                         │ │
│ │ device_id = "{0.0.0.00000000}.{abc-def-...}"   │ │
│ │   ↑ malgo 内部一意 ID (表示名ではない)          │ │
│ └─────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────┘
                  ↑ read on startup
                  ↓ write on change
┌─────────────────────────────────────────────────────┐
│ internal/config/config.go                           │
│ - Load() (*Config, error)                           │
│ - Save() error                                      │
│ - audio.DeviceID (string: malgo 内部 ID)            │
└─────────────────────────────────────────────────────┘
                  ↑ device list (id+name)
                  ↓ selected ID
┌─────────────────────────────────────────────────────┐
│ internal/audio/devices.go (新規)                    │
│ - ListDevices() ([]Device, error)                   │
│ - Device { ID (malgo 内部一意), Name (表示名) }     │
│ - NewCaptureByID(id string) (*Capture, error)       │
└─────────────────────────────────────────────────────┘
                  ↑ click "Refresh" or startup
                  ↓ user selects
┌─────────────────────────────────────────────────────┐
│ internal/tweaks/panel.go (F1 Tweaks)                │
│ - Settings ボタン追加                               │
│ - ドロップダウン ListComboButton (ComboBox)         │
│ - 選択変更 → config.Save() + Capture 再起動         │
└─────────────────────────────────────────────────────┘
```

#### 10.2.4 データ構造

```go
// internal/config/config.go
type Config struct {
    Audio AudioConfig `toml:"audio"`
}

type AudioConfig struct {
    // malgo 内部一意 ID (display name ではない)。
    // ListDevices() で取得した Device.ID を保存。
    // 空文字 = OS デフォルト。
    // 表示名が重複するデバイス (例: 同じ "USB Microphone" が複数)
    // 環境でも ID で一意に識別できる。
    DeviceID string `toml:"device_id"`
}
```

```go
// internal/audio/devices.go
type Device struct {
    // malgo 内部の一意 ID。TOML にはこちらを保存 (S-5)。
    ID string
    // ユーザー向け表示名 ("USB Microphone", "Realtek HD Audio")。
    // UI 表示専用、保存には使わない。
    Name string
}

func ListDevices() ([]Device, error) { ... }
func NewCaptureByID(id string) (*Capture, error) { ... }
```

#### 10.2.5 TOML 保存例

```toml
# os.UserConfigDir()/GoTuber/config.toml
# GoTuber ユーザー設定。GoTuber が自動生成・更新する。
# デバイス ID は malgo 内部の一意 ID。
# 表示名ではなく ID で保存することで、表示名が重複するデバイス
# (例: 同じ "USB Microphone" が複数) 環境でも復元可能。

[audio]
# 使用するマイクデバイス ID。空文字 = OS デフォルト。
device_id = "{0.0.0.00000000}.{abc-def-1234-...}"
```

#### 10.2.6 サブフェーズ分割 (実装手順)

| サブ | 内容 | 工数 |
|---|---|---|
| 1.13a.1 | `internal/config/config.go` 新規 (Load/Save TOML 構造体、`os.UserConfigDir()` でパス取得) | 0.5 日 |
| 1.13a.2 | `internal/audio/devices.go` 新規 (ListDevices / NewCaptureByID) | 0.5 日 |
| 1.13a.3 | `cmd/gotuber/main.go` で config.Load → デバイス ID で audio 初期化 | 0.5 日 |
| 1.13a.4 | `internal/tweaks/panel.go` に Settings ボタン + ドロップダウン追加 | 1 日 |
| 1.13a.5 | ebitenui `ListComboButton` (ComboBox) 統合 + Refresh ボタン | 0.5 日 |
| 1.13a.6 | デバイス変更時のキャプチャ再起動ロジック (P-4: 選択イベントで即座に `config.Save()`) | 0.5 日 |
| 1.13a.7 | ユニットテスト (config TOML round-trip, devices 列挙、ID 一意性) | 0.5 日 |
| 1.13a.8 | code-reviewer + visual test | 0.5 日 |
| **合計** | | **4-5 日** |

#### 10.2.7 完了基準 (DoD)

- [ ] `os.UserConfigDir()/GoTuber/config.toml` が初回起動時に自動生成される
- [ ] Settings → マイク一覧 (表示名) → 選択 → **内部で malgo ID に解決** → 即座にキャプチャが再起動される
- [ ] **P-4: デバイス変更時 (`ListComboButton` 選択イベント) に即座に `config.Save()` が呼ばれ TOML へ書き込まれる**
- [ ] GoTuber 再起動時に選択したマイクが復元される (malgo ID で照合)
- [ ] 存在しないデバイス ID (例: USB 抜いた) を選んだ場合、OS デフォルトにフォールバック + 警告表示
- [ ] config.toml の TOML パース失敗時も OS デフォルトで動作 (graceful degradation)
- [ ] 表示名が重複する複数デバイス環境でも、ID で正しく識別・復元できる
- [ ] `go test ./internal/config/` および `./internal/audio/` 全テスト pass
- [ ] code-reviewer APPROVE
- [ ] yosia さん visual test (F1 → Settings → マイク選択 → 保存 → 再起動で復元確認)

### 10.3 Phase 1.13b: UI 非表示ショートカット (Ctrl+Shift+H)

#### 10.3.1 目標

配信時に Tweaks パネル + 設定ボタンを**全部非表示**にするショートカット。OBS ウィンドウキャプチャに UI が映り込まないようにする。

#### 10.3.2 決定事項 (yosia さんから)

| # | 項目 | 決定 |
|---|---|---|
| 1 | トグル方法 | **Ctrl+Shift+H** で全部の UI を**表示オン/オフ** |
| 2 | 非表示対象 | **Tweaks パネル (F1) + Settings ボタン + 設定ドロップダウン全部** |
| 3 | 表示復帰 | 同じショートカットでもとに戻す (toggle 方式) |

#### 10.3.3 アーキテクチャ

```go
// internal/game/game.go
type Game struct {
    // ... existing
    uiHidden bool // true = 全部の UI を非表示
}

// Update() 内
// P-1: 2 キーは IsKeyPressed (押下状態) + 1 キーは IsKeyJustPressed (立ち上がりエッジ)。
//      3 キー同時 "just pressed" は物理的に検出できない。
ctrl := ebiten.IsKeyPressed(ebiten.KeyControl)
shft := ebiten.IsKeyPressed(ebiten.KeyShift)
hKey := inpututil.IsKeyJustPressed(ebiten.KeyH)
if ctrl && shft && hKey {
    g.uiHidden = !g.uiHidden
}
```

- `g.uiHidden == true` → `Game.Draw()` で `g.panel.Draw(screen)` を呼ばない (Tweaks 表示停止)
- Tweaks 自体が `uiHidden` を見て Settings ボタンも表示しない (F1 押しても何も出ない)
- **終了は OS ウィンドウ閉じる X ボタンで行う** (Phase 1.14 で詳細仕様確定、参照)

#### 10.3.4 サブフェーズ分割

| サブ | 内容 | 工数 |
|---|---|---|
| 1.13b.1 | `Game.uiHidden bool` フィールド + Ctrl+Shift+H 検出 (`IsKeyPressed`(Ctrl+Shift 2 キー) + `IsKeyJustPressed`(H)) | 0.2 日 |
| 1.13b.2 | `Game.Draw()` で `uiHidden` 時に panel.Draw() スキップ | 0.1 日 |
| 1.13b.3 | Tweaks panel 内部で Settings ボタンも uiHidden 反映 | 0.2 日 |
| 1.13b.4 | (削除: kill switch 確認は Phase 1.14 に移動) | - |
| 1.13b.5 | ユニットテスト (uiHidden トグル動作) | 0.2 日 |
| 1.13b.6 | code-reviewer + visual test | 0.2 日 |
| **合計** | | **0.9 日** |

#### 10.3.5 完了基準 (DoD)

- [ ] Ctrl+Shift+H 押下で F1 パネル + 設定 UI が即座に消える
- [ ] もう一度 Ctrl+Shift+H でもとに戻る
- [ ] ~~UI 非表示中も Esc (kill switch) は機能する~~ → **Phase 1.14 で削除予定 (X ボタン終了に統一)**
- [ ] `go test ./internal/game/` 全テスト pass
- [ ] code-reviewer APPROVE
- [ ] yosia さん visual test (OBS ウィンドウキャプチャで UI が映らないこと確認)

### 10.4 リスク・懸念

| # | リスク | 対策 |
|---|---|---|
| 1 | malgo `Devices` API の **デバイス一覧が起動時に空** (WASAPI 遅延初期化) | **P-5: 起動時 1 秒待機 + exponential backoff (1s → 2s → 4s) で最大 3 回 retry。3 回失敗時は OS デフォルトにフォールバック + stderr ログ警告** |
| 2 | ebitenui `ListComboButton` (ComboBox) の **ドロップダウン UI** が Windows 透過ウィンドウで正しく表示されない | 透過ウィンドウでも UI は不透明で描画される (Ebitengine の仕様)。visual test で確認 |
| 3 | **S-3: Ctrl+Shift+H は Windows システムホットキー (最大化/最小化) とは競合しないが、アプリ内ホットキーと競合するリスクあり** (Chrome DevTools: 履歴表示、VS Code: Replace 等) | GoTuber は単一プロセスで Chrome 等のフォアグラウンド判定が必要なため、**最悪 OBS を手前に出して配信続行は可能**。1.13b 後のフィードバック次第で再選定 |
| 4 | **S-2: TOML ライブラリ** | **`github.com/pelletier/go-toml/v2`** (active maintenance、modern API)。`BurntSushi/toml` は 2021 年以降停滞のため**不採用** |
| 5 | 設定変更 → キャプチャ再起動時に **音が一瞬途切れる** | audio バッファをウォームアップしてから切り替える。許容範囲内なら何もしない |
| 6 | **S-5 関連: 表示名が重複する複数デバイス環境で復元失敗する** | **デバイス保存は malgo 内部 ID で**、表示名は UI 表示専用 (S-5 修正で解消) |

### 10.5 リリース順序

1. **Phase 1.13b (UI 非表示ショートカット)** を先に 1 日で完了・リリース
2. 動作確認後、**Phase 1.13a (マイク選択 + 永続化)** に着手 (4-5 日)

理由: 1.13b は独立して動作する機能 (マイク口パク自体は Phase 1.7 で既に動作)、1.13a は audio パッケージ拡張 + UI 統合で 1.13b より規模が大きい。リスクの小さい方を先に。

### 10.6 関連ドキュメント

- `docs/PLAN.md` Section 5 (Phase 1 進捗): Phase 1.13a/b サブ行を追加
- `docs/PLAN.md` Section 2.1 (ライブラリ表): `pelletier-go-toml-v2` を追加 (Phase 1.13a 着手時)
- `docs/新キャラ差し替え手順.md`: 影響なし (キャラクターアセットのみ)
- `README.md` 機能リスト: 「マイクデバイス選択」を Phase 1.13a に追記
- `README.md` キーボードショートカット表: 「Ctrl+Shift+H (UI 全非表示)」を Phase 1.13b に追記

## 11. Phase 1.14: 終了ショートカット削除 (X ボタン + signal.Notify (Unix only) に統一)

### 11.1 背景

Phase 1.13 visual test (2026-06-17) で **致命的なバグ** 発覚:

- **F1 を押すとアプリが即終了する**
- **Esc を押してもアプリが即終了する**
- **ターミナルウィンドウがフラッシングして消える** (Windows Explorer ダブルクリック起動時)

原因分析 (2026-06-17 code-reviewer 検証):

- `killswitch.Install()` で `signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)` を呼んで OS シグナルリッスン
- **当初の仮説**: 「Ebitengine の SIGINT ハンドラと競合」
- **検証結果**: Ebitengine v2.9.9 ソースコード直接確認 → **`signal.Notify` 呼び出しは Ebitengine 内に一切存在しない** (run.go, internal/ui/ui.go, internal/ui/run.go, internal/ui/ui_glfw.go, internal/ui/ui_windows.go, hideconsole 全確認)
- **F1 が `triggered=true` を立てるコードパスは `game.go` / `signal.go` に存在しない** (F1 は PanelVisible トグルのみ、Tick は Esc のみチェック)
- **真因は不明**。候補: ebitenui フォーカス処理、GLFW key event ハンドリング、視覚テスト時の別キー同時押し、別経路の `triggered=true` 設定

追加実機ログ (2026-06-17 PowerShell 起動):

- F1 押下直後の最後のログは `config saved: audio.device_id = ""`
- これは `internal/tweaks/panel.go` の `ListComboButton` 初期選択イベントが `onDeviceSelected("")` を発火し、`cmd/gotuber/main.go` のマイク選択 callback に入った証拠
- `audio restarted with device_id = ""` / `audio restart failed...` が出る前に終了しているため、F1 そのものではなく **F1 → パネル表示 → ComboBox 初期選択 → `Mover.Restart("")` → audio capture lifecycle 破壊** が最有力
- `internal/audio/capture.go` の `NewCaptureByID()` は `defer ctx.Free(); defer ctx.Uninit()` を成功 path にも実行しており、成功して返した `Capture` が解放済み context を持つ可能性がある
- したがって Phase 1.14 は **終了ショートカット削除** に加えて、**マイク ComboBox の初期選択再起動ガード** と **audio context cleanup の成功/失敗 path 分離** を含める

yosia さんの運用判断: **「ウィンドウの閉じる X ボタンで閉じれるから、終了ショートカットはいらない」**。

> **Phase 1.14 の前提条件**: F1 バグの真因調査を **実装着手前** に実施する。真因不明のまま実装すると再発リスク。

### 11.2 目標

- **終了ショートカット (Esc / Q) を削除**。`killswitch.Install()` は **Windows 限定で削除** (`runtime.GOOS == "windows"` ガード)。
- **F1 押下時に ComboBox 初期選択で audio restart しない**。選択済み device ID と同じ場合は save/restart をスキップする。
- **`NewCaptureByID()` の cleanup を成功 path と失敗 path で分離**する。成功時に `ctx.Uninit()` / `ctx.Free()` が走らないようにし、`Capture.Stop()` だけが device/context を解放する。
- **Unix では `signal.Notify` を維持** — Ctrl+C → `triggered=true` → `ebiten.Termination` → 正常終了コード 0 (graceful)
- **Windows では `signal.Notify` を削除** — Ctrl+C は Go runtime デフォルト挙動 (`os.Exit(2)` 即終了、graceful ではない)。GUI モード (hideconsole で FreeConsole 後) は SIGINT が届かない
- **Ctrl+C の挙動**:
  - **Unix**: `signal.Notify` 経由で graceful 終了 (従来通り)
  - **Windows**: Go runtime デフォルトで即終了 (graceful ではない)。推奨終了方法は **X ボタン** (GLFW close callback → `RegularTermination` → graceful)
- **ウィンドウの閉じる X ボタン** を推奨終了方法 (全 OS で graceful)
- `internal/killswitch/` パッケージは **将来用に維持** するが、Windows では `Install()` が no-op になる

### 11.3 変更ファイル

| ファイル | 変更内容 |
|---|---|
| `internal/killswitch/signal.go` | `Install()` に `runtime.GOOS == "windows"` ガード追加 (Windows では no-op)。`Tick()` から `Esc` 検出削除。`escPressed` field を完全削除。`Triggered()` は `triggered.Load()` のみ |
| `internal/game/game.go` | `killswitch.Tick()` 呼び出し削除 (毎フレーム呼ぶ必要なくなる) |
| `internal/tweaks/panel.go:241` | hint テキスト `F1: Toggle Panel  \|  Esc / Q / Ctrl+C: Quit  \|  Ctrl+Shift+H: Hide All UI` → `F1: Toggle Panel  \|  Ctrl+Shift+H: Hide All UI` |
| `internal/tweaks/panel.go:253-257` | Quit ボタンコメント修正: 「killswitch を直接トリガーする」→「Game.Update() で QuitRequested チェック → ebiten.Termination 返却 (Phase 1.14 後の唯一の GUI 終了手段)」|
| `internal/killswitch/signal_test.go` | `TestEscTriggersKillSwitch` 削除 (Esc 検出なくなったため)、`TestTriggeredInitiallyFalse` / `TestSignalTriggersKillSignal` 維持 |
| `internal/audio/capture.go` | `NewCaptureByID()` の cleanup 修正。成功時は context を解放せず、エラー path だけ `Uninit()` → `Free()` する |
| `cmd/gotuber/main.go` / `internal/tweaks/panel.go` | ComboBox 初期選択や同一 device ID 選択では `config.Save()` / `Mover.Restart()` を実行しないガード追加 |
| `docs/PHASE1.md` Section 10.3 系 | kill switch 言及を **strikethrough + Phase 1.14 参照** (本セクションで更新済み) |
| `docs/PLAN.md` Section 7 (kill switch 章) | kill switch 仕様削除、X ボタン + signal.Notify (Unix only) に統一 |
| `docs/README.md` ショートカット表 | `Esc / Q` 削除、X ボタン (全 OS) と Ctrl+C (Unix: graceful / Windows: 即終了) を残す |

### 11.4 サブフェーズ分割 (実装手順)

| サブ | 内容 | 工数 |
|---|---|---|
| 1.14.0 | **F1 バグ真因調査** (実機テスト + ログ採取 + ebitenui/GLFW ソース確認) | 0.3 日 |
| 1.14.1 | `killswitch.Install()` に `runtime.GOOS == "windows"` ガード追加 (Windows では no-op) | 0.2 日 |
| 1.14.2 | `killswitch.Tick()` から `Esc` 検出削除、`escPressed` field を完全削除 (`Triggered()` は `triggered.Load()` のみ) | 0.1 日 |
| 1.14.3 | `internal/game/game.go` から `killswitch.Tick()` 呼び出し削除 | 0.1 日 |
| 1.14.4 | `internal/tweaks/panel.go` hint テキスト更新 + Quit ボタンコメント修正 | 0.1 日 |
| 1.14.5 | ユニットテスト更新 (`TestEscTriggersKillSwitch` 削除) | 0.2 日 |
| 1.14.6 | audio capture lifecycle 修正 (`NewCaptureByID()` success/error cleanup 分離) | 0.3 日 |
| 1.14.7 | ComboBox 初期選択 / 同一 device ID 選択時の save/restart スキップ | 0.3 日 |
| 1.14.8 | code-reviewer + visual test (F1 で Tweaks パネル表示確認、X ボタンでアプリ終了確認) | 0.3 日 |
| 1.14.9 | `firstUpdate` で `applyPassthrough()` を 1 回呼ぶ (X ボタン通過バグ fix) — `internal/game/game.go:97-102` の空だった `firstUpdate` ブロックに `g.applyPassthrough()` を追加。Ebitengine v2 + `ScreenTransparent:true` のデフォルト passthrough=true への暗黙依存を排除し、Phase 1.2 と同じ「起動時 1 回」パターンに戻す。詳細は Section 11.9 | 0.1 日 |
| **合計** | | **1.9 日** |

### 11.5 完了基準 (DoD)

- [ ] **F1 バグの真因が特定・文書化されている** (Phase 1.14.0)
- [ ] F1 押下で Tweaks パネル (Settings ボタン + マイク ComboBox) が表示される (即終了しない)
- [ ] F1 押下直後に `config saved: audio.device_id = ""` が勝手に出ない
- [ ] ComboBox が初期表示された時点では `Mover.Restart("")` が走らない
- [ ] `NewCaptureByID()` 成功後の Capture が解放済み context を保持しない
- [ ] F1 もう一度押下で Tweaks パネルが閉じる
- [ ] Ctrl+Shift+H 押下で全 UI が非表示、もう一度で復帰
- [ ] Ctrl+Shift+H を 5 回連続押下してもアプリが即終了しない
- [ ] Esc 押下は何もしない (F1 表示トグルとも独立)
- [ ] ウィンドウの閉じる X ボタンクリックでアプリが graceful 終了 (全 OS)
- [ ] Ctrl+C (Unix) でアプリが graceful 終了 (`signal.Notify` 経由、終了コード 0)
- [ ] Ctrl+C (Windows, PowerShell から) でアプリが終了 (Go runtime デフォルト、即終了)
- [ ] `go test ./...` 全 pass
- [ ] code-reviewer APPROVE
- [ ] yosia さん visual test (Phase 1.13 visual test の再現手順で確認)
- [ ] **Tweaks パネル表示中にウィンドウの X ボタンクリックでアプリが graceful 終了する** (Phase 1.14.9 — `firstUpdate` で `applyPassthrough()` を 1 回呼ぶ)
- [ ] **Tweaks パネル表示中に最小化・最大化ボタンも反応する** (同上、起動時の passthrough=true を明示確定することで window decoration が反応する)

### 11.6 リスク・懸念

| # | リスク | 対策 |
|---|---|---|
| 1 | **F1 バグの真因が不明** | Phase 1.14.0 で真因調査を必須化。真因不明のまま実装すると再発リスク。候補: ebitenui フォーカス処理、GLFW key event、別キー同時押し |
| 2 | **Windows で Ctrl+C が graceful 終了しない** | 仕様として許容。Windows の推奨終了方法は X ボタン (GLFW close callback → graceful)。Ctrl+C は Go runtime デフォルト (os.Exit(2)) のフォールバック |
| 3 | **`internal/killswitch/` パッケージを完全削除すべき?** | 将来 `syscall.SIGTERM` (Unix) や Windows の CTRL_BREAK_EVENT をハンドルしたくなった時のために、パッケージは **残す** のが安全。`Install()` は Windows で no-op |
| 4 | **Esc キーが将来必要になったら?** | 別フェーズ (Phase 2+) で復活検討。今は削除 |
| 5 | **ComboBox 初期選択イベントが設定保存・マイク再起動を発火する** | 現在選択中 device ID と同じ場合は no-op。初期表示時の自動発火では save/restart しない |
| 6 | **malgo context の cleanup 順序/タイミングを間違えると成功後 Capture が壊れる** | success path では defer cleanup を解除し、error path 専用 cleanup helper で `Uninit()` → `Free()` を保証 |

### 11.7 決定事項 (yosia さんから)

- **A**: 終了ショートカット削除、X ボタンに統一 ← **確定** (2026-06-17)

### 11.8 関連ドキュメント

- `docs/PLAN.md` Section 5: Phase 1.14 行追加
- `docs/PLAN.md` Section 7 (kill switch 章): 削除 or 「signal.Notify (Unix only)」と注記
- `README.md` 機能リスト / ショートカット表: Esc / Q 削除、X ボタン (全 OS) + Ctrl+C (Unix: graceful / Windows: 即終了) を残す
- `CONTEXT_MEMORY.md`: メモリ 1379 参照 (終了ショートカット使用禁止)

### 11.9 Phase 1.14.9: X ボタン通過バグ (visual test で発覚)

#### 症状

Phase 1.14.8 visual test (yosia さん実機) で以下を観測:

- F1 で Tweaks パネル表示後、**ウィンドウの X ボタン (閉じる)** クリックが効かない
- クリックは透過し、ウィンドウが裏に回る (フォーカスを失う)
- 同様に**最小化ボタン・最大化ボタンも反応しない** (透過)

#### 回帰の経緯 (git log -S で追跡)

| コミット | firstUpdate の挙動 | トグル時の挙動 |
|---|---|---|
| `3724564` Phase 1.2 | `ebiten.SetWindowMousePassthrough(true)` 直接呼び出し (常に true) | `SetWindowMousePassthrough(!PanelVisible)` |
| `508f630` Phase 1.13a/b | **空 (呼ばれなくなった)** ← **回帰点** | `applyPassthrough()` (PanelVisible + uiHidden 真理値表) |
| `e38d98b` Phase 1.14 | 空 (変更なし) | 同じ |

**回帰点**: `508f630` Phase 1.13a/b で `applyPassthrough()` を導入した際、`firstUpdate` ブロックから `SetWindowMousePassthrough(true)` の直接呼び出しが消えた。

#### 真因

Ebitengine v2.9.9 の `SetWindowMousePassthrough` は Windows 上で `WS_EX_TRANSPARENT` 相当の拡張スタイルを設定し、**ウィンドウ全体 (タイトルバー含む)** に作用する。

- Phase 1.2: 起動直後 firstUpdate で明示的に `true` をセット → **意図的に X を効かなくしていた** (クリックスルーがデフォルト動作)。ユーザーもそれは仕様と認識していた可能性。
- Phase 1.13+: firstUpdate で何も呼ばない → **Ebitengine v2 + `ScreenTransparent:true` のデフォルト `true` に依存**。F1 押下後に `applyPassthrough()` で `false` をセットするが、Ebitengine v2 の GLFW バックエンドで **`SetWindowMousePassthrough(false)` の効果が遅延する既知ケース** (GitHub Issue #3222 周辺動作) がある。
- 結果: F1 押下後も passthrough=true が持続し、X / 最小化 / 最大化クリックが通過する。

#### 修正方針 (Phase 1.14.9)

**最小修正**: `firstUpdate` ブロックで明示的に `applyPassthrough()` を 1 回呼ぶ。`PanelVisible` と `uiHidden` の初期値は Go のゼロ値 (`false`) なので、起動時は `passthroughDesired(false, false) = true` = クリックスルー有効 = Phase 1.2 と同じ挙動。

```go
if g.firstUpdate {
    g.firstUpdate = false
    // 起動直後の passthrough を明示的に設定。
    // Phase 1.13a/b (508f630) で firstUpdate から SetWindowMousePassthrough 呼び出しが消えた結果、
    // Ebitengine デフォルト (ScreenTransparent:true で passthrough=true) に依存していたが、
    // F1 押下後の SetWindowMousePassthrough(false) の効果が Ebitengine v2 GLFW バックエンドで
    // 遅延するケースがあり X ボタンが通過する症状が出た。
    // applyPassthrough() を 1 回呼ぶことで初期状態を確定させる。
    g.applyPassthrough()

    // 透過ウィンドウの Z-Order は cmd/gotuber/main.go で
    // ebiten.SetWindowFloating(*flagTopmost) を --topmost フラグ (default: false) により
    // 制御する。Ebitengine v2 の透過ウィンドウは OS 仕様で Z-Order が上位に来るため。
}
```

これで:
- 起動直後 PanelVisible=false (Go ゼロ値) → passthrough=true (Phase 1.2 と同じ)
- F1 押下 → applyPassthrough() → passthrough=false (確実)
- **F1 押下以降の X / 最小化 / 最大化ボタンが確実に反応する**

**最小修正の根拠**:
- `firstUpdate` で 1 回呼ぶだけで初期状態が確定し、F1 押下時のトグルと組み合わせて「X が効く」状態へ遷移できる
- 毎フレーム呼ぶ必要はなし (Phase 1.2 と同じパターン)
- Ebitengine デフォルトの不確実性に依存しない、明示的な制御
- コスト: 起動時 1 回のみ

**回帰しない理由**:
- `applyPassthrough()` は `g.tweaks` の PanelVisible / uiHidden を読むだけで副作用なし
- 起動時 PanelVisible=false なので passthrough=true → Phase 1.2 と完全に同じ初期状態
- F1 押下 → PanelVisible=true → passthrough=false (Phase 1.13 の設計通り)

**将来の恒久対策** (Phase 2+ 検討):
- Windows 専用の `WS_EX_TRANSPARENT` を client area のみに限定する (CGo 必須)
- Ebitengine v2 がウィンドウ decoration 用の独立 API を提供するのを待つ
- 現状は YAGNI で firstUpdate 1 回呼び出しで十分

---

## 12. Phase 1.14.10: passthrough 全面廃止 (X ボタン常時有効化) — **2026-06-17 確定**

### 背景

Phase 1.14.9 firstUpdate fix 後も **F1 パネル非表示時に X ボタンが通過する**ことが yosia さん実機 visual test で発覚。ホバーしても赤く光らない (Windows から見るとウィンドウがマウスに対して完全透明扱い)。

`SetWindowMousePassthrough(true)` = `WS_EX_TRANSPARENT` = ウィンドウ全体 (タイトルバー含む) クリック透過。Ebitengine v2.9.9 の純粋な API では per-region passthrough は不可能。Win32 API (CGo) は工数大、FramelessWindow 自前タイトルバー描画は工数中。

### 決定

**`passthroughDesired` を常に `false` 返す** = passthrough 全面廃止。X ボタン / 最小化 / 最大化が常に効く。

**犠牲**: OBS クリック透過 (キャラ部分クリックが背後のウィンドウに届かない)。`ScreenTransparent: true` は維持するので OBS ウィンドウキャプチャは引き続き可能 (背景透過は維持)。

### 修正

`internal/game/game.go` の `passthroughDesired` を変更:

```go
// 旧:
// func passthroughDesired(panelVisible, uiHidden bool) bool {
//     return !(panelVisible && !uiHidden)
// }

// 新 (Phase 1.14.10):
func passthroughDesired(panelVisible, uiHidden bool) bool {
    return false
}
```

`applyPassthrough()` と Phase 1.14.9 firstUpdate 呼び出しはそのまま残す (冗長だが、Phase 2+ でクリック透過を復活する時のアンカーとして機能)。

### 影響範囲

- ✅ X ボタン / 最小化 / 最大化が常に効く
- ✅ タイトルバーホバーでハイライト表示
- ✅ Tweaks パネル内クリック受付 (PanelVisible=true 時は元から passthrough=false)
- ⚠️ キャラ部分クリックが背後に届かない (OBS クリック透過廃止)
- ⚠️ ユーザーはキャラ上で右クリック等のデスクトップ操作不可

### DoD

- [ ] yosia さん visual test: F1 パネル非表示時に X ボタンクリックでアプリ終了
- [ ] X ボタン / 最小化 / 最大化にマウスを乗せると Windows 標準のハイライト表示 (赤・青・黄)
- [ ] F1 でパネル表示 → X / 最小化 / 最大化クリックで全て反応する (Phase 1.14.9 + 1.14.10)
- [ ] `go test ./...` 全 pass
- [ ] code-reviewer APPROVE

