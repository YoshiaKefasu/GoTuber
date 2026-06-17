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

---

## 13. Phase 1.14.14: adaptive noise gate (Mouth 不感症対策)

### 13.1 背景

Phase 1.14.13 で Tweaks パネルの debug 表示 (RMS / Envelope / Mouth) を実装したが、
yosia さんの実機 (Windows) で「全マイクデバイスを試しても口パクしない」症状を観測:

```text
Audio RMS: 0.0038 | Envelope: 0.0091 | Mouth: closed
```

原因は thresholdMouth0=0.05 (closed→half) と thresholdMouth1=0.20 (half→open) の
**固定閾値**が実機マイクの RMS レンジと合っていなかった。固定閾値を雑に下げると、
今度は無音ノイズで口がパクパクする。

「環境ノイズで口パク」と「小声で口パク」の両方を成立させるには、固定閾値では
なく **adaptive** な閾値が必要。

### 13.2 方針 (YAGNI)

採用: **adaptive noise floor + gate hysteresis + gain** の 3 段。

非採用 (YAGNI、複雑化が費用に見合わない):
- **AI VAD** (silero / webrtcvad) — モデル依存 + 数百 ms 遅延
- **WebRTC VAD** — C ライブラリ依存 (CGo 重い)
- **FFT / spectral subtraction** — 計算コスト
- **AGC** — 過剰反応、ゲイン暴走リスク

### 13.3 変更ファイル

| ファイル | 変更内容 |
|---|---|
| `internal/audio/mover.go` | `Mover` に `noiseFloor / gateOpen / sensitivity / noiseWarmupFrames` 4 フィールド追加。`Metrics` 構造体新設。`UpdateWithMetrics()` 戻り値を tuple → `Metrics` に変更。`applyNoiseGate()` private メソッド新設。`defaultMicSensitivity = 15.0` |
| `internal/audio/envelope.go` | 変更なし (`EnvelopeFollower` / `MouthTracker` はそのまま) |
| `internal/audio/mover_test.go` | noise gate テスト 8 件追加 (`TestNoiseGate_*`, `TestUpdateWithMetrics_*`) |
| `internal/tweaks/state.go` | `AudioNoiseFloor / AudioGatedRMS / AudioGateOpen` 3 フィールド追加 |
| `internal/tweaks/panel.go` | `audioDebugText2 *widget.Text` フィールド追加 (2 行表示化)。`audioDebugLabel1/2()` / `gateStateLabel()` 関数新設 |
| `internal/tweaks/panel_test.go` | `TestAudioDebugLabel` → `TestAudioDebugLabel1/2/Labels_GateClosed` 分割。`TestGateStateLabel` 追加 |
| `internal/game/game.go` | `g.audio.UpdateWithMetrics()` 戻り値 Metrics 6 フィールドを `tweaks.State` に展開 |
| `docs/PLAN.md` | Phase 1.14.14 行追加 |
| `docs/PHASE1.md` | 本セクション追記 |

### 13.4 アルゴリズム (シンプル、YAGNI)

#### 13.4.1 処理順

```text
raw RMS → applyNoiseGate → gated RMS → envelope (attack/release) → mouth (3 状態)
```

#### 13.4.2 applyNoiseGate の中身

1. **noise floor 学習 (warmup)**: 起動直後 `noiseFloorWarmupFrames = 60` フレーム
   (約 1 秒) は gate を強制 closed にして環境ノイズだけを先に学習する。
   理由: `noiseFloor=0` 初期値のままだと最初の非ゼロ raw サンプルで
   `raw > floor+openMargin` が即発火し、常時ノイズを voice と誤認する。

2. **noise floor 追従**: gate closed 期間中のみ raw RMS に向けて exponential
   filter で追従 (`noiseFloorRiseRate=0.02` / `noiseFloorFallRate=0.08` per update)。
   gate 開放中は floor を凍結し、voice を noise として学習しない。

3. **gate ヒステリシス**: `gateOpenMargin=0.002` / `gateCloseMargin=0.001` の
   マージン差で境界フリッカ抑制。`raw > floor+open` で開、`raw < floor+close` で閉じる。

4. **gain**: gate 開放中は `(raw - floor - closeMargin) * sensitivity` を [0, 1] に
   クランプして返す。`sensitivity=0` の初回は `defaultMicSensitivity=15.0` を lazy 初期化。

5. **gate closed**: 0 を返す (envelope は release でゆっくり減衰)。

#### 13.4.3 採用した初期値 (実機 tweak 用、visual test で再調整前提)

| 定数 | 値 | 理由 |
|---|---|---|
| `defaultMicSensitivity` | 15.0 | raw 0.005 でも 0.07 (MouthHalf) を超えるゲイン |
| `noiseFloorRiseRate` | 0.02 | 環境ノイズ上昇時はゆっくり追従 |
| `noiseFloorFallRate` | 0.08 | 環境ノイズ下降時は速めに追従 |
| `noiseFloorWarmupFrames` | 60 | 60fps 前提で約 1 秒。gate 即開き回避 |
| `gateOpenMargin` | 0.002 | 開側のマージン |
| `gateCloseMargin` | 0.001 | 閉側のマージン (open より小さく = 短発話で gate を開いたままにする) |

### 13.5 API 変更 (局所)

`Mover.UpdateWithMetrics()` 戻り値が `(rms, env float64, mouth int)` から
`Metrics` 構造体 (`RMS / NoiseFloor / GatedRMS / Envelope / Mouth / GateOpen` の 6
フィールド) に変更。**呼び出し元は `game.go:165` の 1 箇所のみ**なので影響局所。
既存の `Update() int` (口パク状態のみ返す) は無変更で互換維持。

### 13.6 Tweaks パネル debug 表示 (2 行化)

```text
Audio RMS: 0.0084 | Floor: 0.0020 | Gate: open
Gated: 0.0420 | Envelope: 0.0310 | Mouth: closed
```

読み方:
- `RMS=0` → マイク入力自体が来ていない (device 不正 / ミュート)
- `RMS>0, Floor 同程度, Gate closed` → 環境ノイズのみ (正常)
- `Gate open なのに Mouth が動かない` → gain / envelope 閾値側を疑う
- `Gate closed なのに Mouth 動く` → ロジック破壊、要調査

### 13.7 実機データ (Phase 1.14.15 への引き継ぎ)

```text
Audio RMS: 0.0084 | Floor: 0.0020 | Gate: open
Gated: 0.0810 | Envelope: 0.1338 | Mouth: half
```

計算: `(0.0084 - 0.0020 - 0.001) * 15 = 0.081` → MouthHalf 閾値 0.07 超え。
**無音時にも敏感すぎて口が半開きになる** という別問題が発生。Phase 1.14.15 で
Mic Sensitivity slider 導入 (= `defaultMicSensitivity` を 15→10 に下げる +
UI で調整可能化) で解決。

### 13.8 DoD (Phase 1.14.14 完了基準)

- [x] `Metrics` 構造体 6 フィールド (RMS / NoiseFloor / GatedRMS / Envelope / Mouth / GateOpen)
- [x] `applyNoiseGate` で adaptive noise floor + gate hysteresis + gain
- [x] 起動 60 frame の warmup で gate 即開き回避
- [x] Tweaks パネル 2 行 debug 表示 (raw / gated)
- [x] `go test ./...` 全 pass (Phase 1.14.14 時点で 7 パッケージ全 pass)
- [x] yosia さん実機 visual test で口パク動作確認 (Phase 1.14.15 で感度調整必要判明)

---

## 14. Phase 1.14.15: Mic Sensitivity slider (無音ノイズで口パクしすぎる問題)

### 14.1 背景

Phase 1.14.14 で口パクは動くようになったが、yosia さん実機 visual test で **喋っていない時も口が半開きになる**ことを確認した。

実機値:

```text
Audio RMS: 0.0084 | Floor: 0.0020 | Gate: open
Gated: 0.0810 | Envelope: 0.1338 | Mouth: half
```

計算:

```text
(0.0084 - 0.0020 - 0.001) * 15 = 0.081
```

`MouthClosed -> MouthHalf` 閾値は `0.07` 超えなので、15x では無音ノイズでも口が半開きになる。

### 14.2 決定

- `defaultMicSensitivity` を **15.0x → 10.0x** に下げる
- Tweaks パネルに **Mic Sensitivity slider** を追加する
- 範囲は **1.0x..20.0x**、0.1x 刻み
- TOML 永続化はまだしない。まず visual test で実機値を探る

### 14.3 UI

Tweaks パネルの `Mic Mouth Movement` の下に追加:

```text
Mic Sensitivity: 10.0x
[ slider ]
```

### 14.4 実装メモ

| ファイル | 変更 |
|---|---|
| `internal/audio/mover.go` | `SetSensitivity(value float64)` 追加。1.0x..20.0x に clamp。`defaultMicSensitivity=10.0` |
| `internal/tweaks/state.go` | `AudioSensitivity float64` 追加。default 10.0 |
| `internal/tweaks/panel.go` | slider + dynamic label 追加 |
| `internal/game/game.go` | `audio.UpdateWithMetrics()` 前に `SetSensitivity(state.AudioSensitivity)` |

### 14.5 Visual test の見方

| 状態 | 期待 |
|---|---|
| 無音で口が動く | slider を下げる。`Gated` / `Envelope` が下がるはず |
| 発話しても口が動かない | slider を上げる。`Mouth: half/open` になる値を探す |
| ちょうどよい値 | 無音は `Mouth: closed`、発話は `half/open` |

### 14.6 DoD

- [x] Tweaks に `Mic Sensitivity: 10.0x` と slider が表示される
- [x] slider 操作で debug の `Gated` / `Envelope` が変わる
- [x] 無音時の口パクを抑えられる
- [x] 発話時は口パクする
- [x] `go test ./...` 全 pass
- [x] `go vet ./...` pass
- [x] Windows build success
- [x] code-reviewer APPROVE

---

## 15. Phase 1.14.16: Tweaks 永続化 (再起動後リセット問題) — 明示的 Save ボタン方式

### 15.1 背景

Phase 1.13a で `audio.device_id` だけが TOML (`%APPDATA%\GoTuber\config.toml`) に保存される。  
yosia さん実機 visual test で **Mic デバイス以外が全部再起動で揮発する** ことが判明:

| Tweaks 項目 | 再起動で |
|---|---|
| Mouse Follow slider | **消える** |
| Auto Blink ON/OFF | **消える** |
| Mic Mouth Movement ON/OFF | **消える** |
| Mic Sensitivity slider | **消える** |
| Mic device selection | ✅ 残る (Phase 1.13a で対応済) |

ユーザー報告: 「現状では十分だけど、その設定は再起動後でリセットされるだけです。たぶん保存ボタンを追加すればいいでしょ？保存を押さない限り適応されないにすると、保存ボタンを押したら変更をフラッシュ書き込むWriteです。起動の時にその設定確認あればロードするとなければデフォルトにします。」

**本 Phase は passthrough / X ボタン挙動に影響しない** (Phase 1.14.10 の `passthroughDesired = false` 状態を維持)。

### 15.2 決定: 明示的 Save ボタン方式 (Reset ボタンは YAGNI 削除)

**自動 Save は採用しない**。代わりに Tweaks パネルに `Save` ボタンのみを追加する:

| 操作 | 挙動 |
|---|---|
| スライダー・チェックボックス変更 | `state.Dirty = true` を立てるだけ。**TOML には書かない** |
| `Save` ボタン押下 | `state.Dirty == true` なら TOML に書き込む → ボタン disable / `statusLabel` を `saved` に |
| Quit / X ボタンで終了 | **dirty 関係なく保存しない** (Save 押してない変更は失われるのが仕様) |
| 設定をデフォルトに戻したい | TOML ファイル (`%APPDATA%\GoTuber\config.toml`) を手動削除 → 次回起動で NewState() デフォルトから再開 |

**Reset ボタンは Phase 1.14.16 Round 3 で YAGNI 削除**。理由:
- ebitenui v0.7.3 の `widget.Slider` には `SetCurrent(int)` が無く、`lastCurrent` フィールドも private
- 手動 `ChangedEvent.Fire()` 後に slider の `Render()` が `s.Current != s.lastCurrent` で再 fire する
- `mutingSliders` フラグは手動 fire 経路のみ mute でき、次フレーム Render の再 fire は止められない
- `RefreshWidgetsFromState()` の根本修正には ebitenui への upstream PR / fork / 代替ライブラリ切り替えが必要 (YAGNI 過剰)
- 結果: Reset 機能の UX 価値 (「実験的なスライダー位置から defaults に戻す」) と、複雑性 (CheckBox `tri-state` guard + Slider `ChangedEvent.Fire` 模倣 + mute 機構 + Checkbox 同期も同じ問題) のトレードオフで、YAGNI 削除が最善
- 代替: デフォルトに戻したいケースでは TOML ファイル削除で次起動から defaults (yousa さんがこの運用で OK と承認)

UI フィードバック:

```text
[ Mouse Follow slider ... ]
[ Auto Blink  [x] ]
[ Mic Mouth Movement  [x] ]
[ Mic Sensitivity: 10.0x [ slider ] ]
[ Mic Device: (OS default) [v] Refresh ]

[ Save ]
status: "unsaved changes"   ← statusLabel に dirty=true なら表示
```

ボタン活性ルール:

- `Save` ボタン: `state.Dirty == true` のときのみ enable。Save 成功後 disable (`Dirty = false`)
- Reset ボタンは存在しない (Section 15.2 の YAGNI 削除)

### 15.3 TOML 構造

```toml
[audio]
device_id = "..."

[tweaks]
mouse_responsiveness = 0.3
blink_enabled = true
mouth_enabled = true
mic_sensitivity = 10.0
```

- 保存先: `os.UserConfigDir()/GoTuber/config.toml` (既存と同じ、`Path()` を再利用)
- ライブラリ: `github.com/pelletier/go-toml/v2` (既存)
- **永続化しない値**:
  - `tweaks.State` 内: `PanelVisible` (state.go:35)、`QuitRequested` (state.go:38)、`Dirty` (Save 後に false に戻す)、Audio debug 6 フィールド (`AudioRMS / AudioNoiseFloor / AudioGatedRMS / AudioEnvelope / AudioMouth / AudioGateOpen` — `state.go:21-32`)
  - `Game` 構造体内: `uiHidden` (game.go:58、Ctrl+Shift+H の配信モード、起動時は表示に戻すのが安全)
- **TOML セクション欠落 vs ゼロ値区別**: ゼロ値 (`0.0` / `false`) を「未設定」とみなし `ApplyTo` で skip。`MouseResponsiveness=0.0` のまま TOML に書き戻す事故を防ぐ。Phase 1.13a の `Audio.DeviceID == ""` を "OS default" 扱いする方針と一貫。

### 15.4 起動時ロード順序 (cmd/gotuber/main.go)

```text
1. character.LoadConfig("config/default.yaml")
2. config.Load() → userCfg
3. NewState() でデフォルト値の tweaksState を作成
4. userCfg.Tweaks.ApplyTo(tweaksState)
   - 各フィールドがゼロ値 → skip (デフォルトのまま)
   - ゼロ値以外 → 上書き
5. panel := NewPanel(face, tweaksState, mover != nil, userCfg.Audio.DeviceID)
   → 初期表示に userCfg.Audio.DeviceID を ComboBox 選択に反映
   → Slider/Checkbox 初期値は tweaksState から読む (すでに ApplyTo 済み)
6. (以降 Phase 1.13a と同じフロー: audio.NewMoverByID / devicesCh / Game.New)
```

**ComboBox 初期選択同期** (Section 15.6 の `NewPanel` シグネチャ拡張とセット):

```go
// ListComboButton.SetSelectedEntry を使う
for i, e := range comboEntries {
    if dev, ok := e.(audio.Device); ok && dev.ID == userCfg.Audio.DeviceID {
        panel.micCombo.SetSelectedEntry(e)
        break
    }
}
```

ユーザーが起動時 ComboBox で「保存済みデバイス名」を視覚確認できる。

### 15.5 Save 動作詳細

```go
// === TweaksConfig 構造体 (新規) ===
// Phase 1.14.16 (Critical #2 fix): BlinkEnabled / MouthEnabled は *bool として TOML に書き出す。
// TOML に「キーが存在しない」(nil) と「キーが存在するが false」 (非nil, false) を区別するため。
// ゼロ値 bool では区別不能で、初回起動時にユーザーが明示的に OFF を選択していないのに
// 口パク・まばたきが無効化されるバグが発生する。
type TweaksConfig struct {
    MouseResponsiveness float64 `toml:"mouse_responsiveness"`
    BlinkEnabled        *bool   `toml:"blink_enabled"`
    MouthEnabled        *bool   `toml:"mouth_enabled"`
    MicSensitivity      float64 `toml:"mic_sensitivity"`
}

// ApplyTo は TOML から読んだ値を state に上書きする。
// ゼロ値 / nil は「TOML に書かれていない (= 未設定)」とみなして skip。
//   - MouseResponsiveness: 0.0 なら skip (state のデフォルト 0.3 を保持)
//   - BlinkEnabled: *bool が nil なら skip (state のデフォルト true を保持)、
//     非 nil なら *b の値を State に上書き (明示的 OFF を尊重)
//   - MouthEnabled: *bool が nil なら skip (state のデフォルト true を保持)、
//     非 nil なら *b の値を State に上書き (明示的 OFF を尊重)
//   - MicSensitivity: 0.0 なら skip (state のデフォルト 10.0 を保持)
//
// 呼び出しは cmd/gotuber/main.go の config.Load() 直後、NewState() 直後。
func (t *TweaksConfig) ApplyTo(state *tweaks.State) {
    if t.MouseResponsiveness != 0 { state.MouseResponsiveness = t.MouseResponsiveness }
    if t.BlinkEnabled != nil { state.BlinkEnabled = *t.BlinkEnabled }
    if t.MouthEnabled != nil { state.AudioEnabled = *t.MouthEnabled }
    if t.MicSensitivity != 0 { state.AudioSensitivity = t.MicSensitivity }
}

// CaptureFrom は state の 4 フィールドを TOML 書き込み対象としてコピーする。
// Save ボタン押下時に main.go から呼ばれる。
//
// Phase 1.14.16 (Critical #2 fix): BlinkEnabled / MouthEnabled は *bool として必ずコピー
// (nil にしない)。Save ボタン押下 = ユーザーが明示的に Save を選択した瞬間なので、
// 「明示的 OFF」と「TOML 欠落」を区別する必要はない。
func (t *TweaksConfig) CaptureFrom(state *tweaks.State) {
    t.MouseResponsiveness = state.MouseResponsiveness
    blinkVal := state.BlinkEnabled
    t.BlinkEnabled = &blinkVal
    mouthVal := state.AudioEnabled
    t.MouthEnabled = &mouthVal
    t.MicSensitivity = state.AudioSensitivity
}

// === Panel UI ===
// Save ボタンは main.go 側でオーケストレート。
// SetOnSaveRequested のみ存在 (SetOnResetRequested は Round 3 で YAGNI 削除)。
// RefreshWidgetsFromState も Round 3 で YAGNI 削除 (ebitenui slider の lastCurrent
// private 問題が構造的に解決できないため)。
func (p *Panel) SetOnSaveRequested(fn func() error)
func (p *Panel) SetStatus(message string)        // statusLabel.Label を更新

// === Save 押下時 (main.go) ===
panel.SetOnSaveRequested(func() error {
    cfg.Tweaks.CaptureFrom(state)
    if err := cfg.Save(); err != nil {
        panel.SetStatus("save failed: " + err.Error())
        return err  // ボタン状態は dirty のまま
    }
    state.Dirty = false
    panel.SetStatus("saved")
    return nil
})

// === Reset 押下時 (YAGNI 削除済み) ===
// Phase 1.14.16 Round 3 で Reset ボタンは削除された。
// 詳細は Section 15.2 の YAGNI 削除理由を参照。
```

#### 15.5.1 Slider Reset の制約 (ebitenui v0.7.3) — YAGNI 削除済み

Phase 1.14.16 Round 2 で `RefreshWidgetsFromState()` 実装時に発見した制約だが、Round 3 で Reset 機能自体が YAGNI 削除されたため、対応不要。

参考情報 (将来別 Phase で Reset を再実装する場合に参照):

- ebitenui v0.7.3 の `widget.Slider` には `SetCurrent(int)` メソッドは **存在しない** (`widget/slider.go` 実ソース確認)
- `ProgressBar.SetCurrent()` は存在するが Slider には無い
- 公開フィールドは `Min / Max / Current / DrawTrackDisabled` のみ
- `lastCurrent` フィールドは private で外部から更新不可
- 手動 `ChangedEvent.Fire()` は `lastCurrent` を更新しないため、次フレーム `Render()` が `s.Current != s.lastCurrent` で再 fire する
- 完全な修正には ebitenui への upstream PR (SetCurrent + lastCurrent setter 公開) または fork が必要

#### 15.5.2 Checkbox tri-state guard — 構築時のみ

`audioEnabled=false` で起動すると Mic Mouth Movement checkbox は `WidgetGreyed` で初期化される。  
このとき `widget.CheckboxOpts.TriState()` を指定しておかないと、構築時の `Validate()` で `widget/checkbox.go:116-118` の `panic("non-tri state Checkbox cannot be in greyed state")` が発生する。

Round 3 で `RefreshWidgetsFromState()` を削除したので SetState を呼ぶ箇所はなくなったが、構築時の panic 回避のため `TriState()` は残す (Phase 1.14.16 Critical #1 fix)。

### 15.6 実装メモ

| ファイル | 変更 |
|---|---|
| `internal/config/config.go` | `Config.Tweaks TweaksConfig` 追加。`TweaksConfig` は 4 フィールド。`BlinkEnabled` / `MouthEnabled` は `*bool` で TOML キー欠落 (nil) と false 区別 (Critical #2 fix)。`ApplyTo(state *tweaks.State)` / `CaptureFrom(state *tweaks.State)` ヘルパ追加。ゼロ値 / nil skip 方針 |
| `internal/tweaks/state.go` | `Dirty bool` フィールド追加。`ResetToDefaults()` メソッドは後方互換のため残す (Round 3 で Reset ボタン削除済み) |
| `internal/tweaks/panel.go` | 3 つの slider/checkbox ChangedHandler で `p.state.Dirty = true` を立てる。`Save` ボタン + `statusLabel *widget.Text` 追加。`statusLabel.Label = statusText(p.state)` を毎フレーム更新。`SetOnSaveRequested(fn func() error)` / `SetStatus(string)` 公開メソッド。`NewPanel` に `initialDeviceID` 引数追加 + `widget.CheckboxOpts.TriState()` 追加 (Critical #1 fix)。`SetDevices` で ComboBox 初期選択同期。Reset ボタン / `RefreshWidgetsFromState` / `SetOnResetRequested` は Round 3 で YAGNI 削除 |
| `cmd/gotuber/main.go` | `config.Load()` 後に `userCfg.Tweaks.ApplyTo(tweaksState)` を呼ぶ。`NewPanel` シグネチャ拡張: `NewPanel(face, state, audioEnabled, initialDeviceID string)`。ComboBox 初期選択を `userCfg.Audio.DeviceID` に同期。`panel.SetOnSaveRequested` 設定 (Reset オーケストレーション削除) |
| `internal/config/config_test.go` | TOML round-trip テスト追加。`*bool` の nil skip / 明示的 false / CaptureFrom 全 nil 化なし、3 テスト追加 |
| `internal/tweaks/panel_test.go` | Save ボタン初期 disable テスト、statusLabel テスト、SetStatus テスト、audioEnabled=false tri-state 構築テスト追加 (RefreshWidgetsFromState / mute 関連テストは Round 3 で削除) |
| `docs/PHASE1.md` / `docs/PLAN.md` | 本セクション + Section 5 に 19 番目エントリ追加 (Round 3 で Reset 関連の記述を YAGNI 削除済み) |

### 15.7 想定の TOML 例 (yosia さんの環境)

```toml
[audio]
device_id = "7b0030002e0030002e0031002e003000300030003000300030003000000700d00e007b006500610039006400650066006000300034002d0061006100380038002d003400600610062002d003803700640035002d00320032003200350035003300370003003400620063063006306300630038007d"

[tweaks]
mouse_responsiveness = 0.3
blink_enabled = true
mouth_enabled = true
mic_sensitivity = 10.0
```

### 15.8 Phase 1.14.14 noise gate との相互作用

Section 15.3 の Audio debug 6 フィールド (`AudioRMS / AudioNoiseFloor / AudioGatedRMS / AudioEnvelope / AudioMouth / AudioGateOpen`) は:

- **Tweaks パネルで観察・診断のみ** (Phase 1.14.14 で追加)
- Save / Reset / 起動時ロードのいずれにも **影響しない**
- 再起動のたびに `audio.Mover` の adaptive filter が 60 frame warmup からやり直し

ユーザーが「Save 押せば Noise Floor も保存される」と誤解しないよう、Section 15.8 (Visual test の見方) に明示する。

### 15.9 Visual test の見方

| 操作 | 期待 |
|---|---|
| F1 → Mouse Follow slider を 0.8 へ | スライダーは 0.8。Save 押すまで TOML は変わらず、status に `unsaved changes` |
| F1 → Save 押下 | status が `saved` に変わる。`%APPDATA%\GoTuber\config.toml` の `mouse_responsiveness` が `0.8` になっている |
| Quit / X ボタンで終了 → 再起動 | Mouse Follow = 0.8 が復元される |
| F1 → Auto Blink を OFF → Quit / X | dirty な変更は破棄。次回起動時 Auto Blink = ON (default)。Save 押してない設定は再起動で消えるのが仕様 |
| Mic デバイス切替 | 引き続き即時 TOML 保存 (Phase 1.13a の挙動を維持 — Mic は特殊。Save ボタン待たずに書く) |
| 起動 → ComboBox 表示 | 保存済みデバイス名が選択状態で表示される (Phase 1.13a で audio runtime には反映済みだったが ComboBox 視覚には反映されていなかった点を修正) |
| 設定を defaults に戻したい | `%APPDATA%\GoTuber\config.toml` を手動削除 → 次回起動で `NewState()` デフォルトから再開 (YAGNI で Reset ボタン削除済み) |

### 15.10 DoD

- [x] Tweaks に `Save` ボタンと `unsaved changes` status が表示される (Reset ボタンは YAGNI 削除済み)
- [x] Save ボタンが `state.Dirty` で enable/disable する
- [x] Save 押下で TOML に書き込まれ、status が `saved` に変わる
- [x] Quit / X ボタン押下時に dirty な変更は破棄される (Save 押してない設定は再起動で消える)
- [x] Mic デバイスは Save ボタン待たず即時保存される (Phase 1.13a 互換)
- [x] 起動時 ComboBox が保存済みデバイス名で初期選択される
- [x] TOML に `[tweaks]` セクションが追加される
- [x] Tweaks の 4 項目すべてが再起動後に復元される
- [x] TOML のゼロ値 / nil は ApplyTo で skip される (`*bool` で TOML キー欠落区別)
- [x] `PanelVisible` / `uiHidden` / debug 値は再起動でリセットされる (仕様通り)
- [x] `go test ./...` 全 pass (Phase 1.14.16 Round 3 時点で 7 パッケージ全 pass)
- [x] `go vet ./...` pass
- [x] Windows build success
- [x] code-reviewer APPROVE (Round 3 で YAGNI 削除方針承認待ち)
- [ ] yosia さん実機 visual test で再起動後復元確認
