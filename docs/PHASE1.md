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

- [ ] `internal/character/config.go` を camelCase YAML tag に書き換え
- [ ] `config/default.yaml` を camelCase スキーマに書き換え
- [ ] `internal/mouse/follow.go:Cell()` の Y 軸反転を削除
- [ ] `mouse/follow_test.go` の期待値を新仕様 (r0=上) に更新
- [ ] `tools/slice_character_sheets.py` を元 648 行版に置換
- [ ] `tools/slice_character_sheets_test.py` を元版に置換
- [ ] `tools/requirements.txt` に ffmpeg/ffprobe への言及追加
- [ ] `tools/genplaceholder/` ディレクトリを削除
- [ ] `assets/characters/_default/` の 150 枚を 1200×1200 WebP に再生成
- [ ] ビルドスクリプト (`scripts/build.ps1` / `scripts/build.sh`) で動作確認

#### ドキュメント

- [ ] `docs/新キャラ差し替え手順.md` を「100% port」明記に書き換え
- [ ] `docs/PHASE1.md` Section 9 を全面書き換え (port vs 互換性)
- [ ] `docs/PLAN.md` 機能マッピング表を `character-config.js` ベースに更新
- [ ] `docs/PLAN.md` 設定例を camelCase に更新
- [ ] `docs/PLAN.md` Phase リストに Phase 1.12 (port) として明記
- [ ] `README.md` キャラ作成手順を新仕様に更新
- [ ] `README.md` で「Phase 1.11 Done / Phase 1.12 Planned」と状態明示

#### 検証

- [ ] `go test ./...` 全パス
- [ ] `go build` (Windows + Linux) 成功
- [ ] 起動時ログで「mouse Y-axis flip removed (matches tomari-guruguru app.jsx:62)」と表示
- [ ] 起動時ログで「config keys: basePath, ext, rows, cols, sheets.{eyesOpen,eyesClosed}.{close,half,open}」と表示
- [ ] スライスツール単体テスト (`python -m pytest tools/slice_character_sheets_test.py`) 全パス
- [ ] 元 `public/slices2/` の中身を `assets/characters/_default/` にコピーして起動できる (drop-in 互換確認)

### 9.9 互換性確認 (drop-in)

Phase 1.12 完了後、元 tomari-guruguru のスライス済みデータ (4500×4500 PNG を component mode で
25 枚に分割した結果) が、GoTuber 側にドロップインで動く:

- **ファイル命名規則**: `r{行}c{列}.{ext}` → 一致
- **ディレクトリ構造**: `{basePath}/{A-F}/r{0-4}c{0-4}.{ext}` → 一致
- **設定**: `src/character-config.js` の `basePath` + `ext` + `sheets` → GoTuber の `config/default.yaml` の `basePath` + `ext` + `sheets` で対応
- **マウス追従**: `r0=上` で完全一致

つまり、**Phase 1.11 で生成した 100x100 PNG プレースホルダは破棄**し、**元プロジェクトのスライス済み WebP を `assets/characters/_default/` にそのままコピー**すれば、GoTuber で動作する。
