# GoTuber — Phase 3: Creator Tools 詳細設計

> **ステータス**: Phase 3.0 仕様固定完了（2026-06-24）
> **最終更新**: 2026-06-24
> **親プラン**: [PLAN.md](./PLAN.md) v0.4.8

---

## 1. 目標

Phase 3 は **GoTuber Creator Tools** とする。

目的は、初回ユーザーが Live2D モデルを作らずに、**1 枚のベース画像から GoTuber 用キャラクター素材を作り始められる**制作支援ツールを整えること。

GoTuber は外部 VTuber アプリへモーションを渡すフレームワークではなく、**GoTuber 内で完結する低コスト PNGTuber 制作フレームワーク**として育てる。

---

## 2. 背景

Phase 1 / Phase 2 で GoTuber 本体は以下を満たした。

- 150 枚 WebP (`A-F` × `5x5`) を読み込んで表示できる
- マイク口パク、瞬き、マウス追従が動く
- MediaPipe カメラ追従も動く
- Windows native camera build も成立

次に詰まるのは「キャラクター素材をどう作るか」。

従来案の「6 枚表情シートを全部 AI で先に作る」方式は、初回ユーザーには重い。Phase 3 では、まず **目開き + 口閉じのメイン画像 1 枚**から始める。

---

## 3. ゴール / 非ゴール

### 3.1 ゴール

- 1 枚のメイン画像を入力して、A 状態 (`eyesOpen.close`) の 25 枚を作る
- 入力画像を必要に応じてアップスケールする
- 5×5 グリッドに切り出す
- 各セルに対して **目と眉毛マスク** / **口マスク** を作る
- 赤マスク PNG でユーザーがレビューできる
- 将来的に PSD / KRA / ORA 等のレイヤー付き出力も検討できる構造にする
- ユーザーが ChatGPT / Codex / 画像生成 AI を使って、A の 25 枚を高品質化しやすい導線を作る
- 高品質 A 25 枚 + マスクを元に、B〜F を Inpaint で作るワークフローを明文化する

### 3.2 非ゴール

- Phase 3 初期で完全自動 150 枚生成までは狙わない
- Live2D / VRM / VMC 連携は扱わない
- 外部 VTuber ソフトへのモーション送信は扱わない
- 画像生成 AI の API 呼び出しは初期スコープ外
- ユーザーの画風を完全自動で保証しない

---

## 4. 用語

| 用語 | 意味 |
|---|---|
| A | 目開き + 口閉じ。GoTuber の基準表情 |
| B | 目開き + 口半開き |
| C | 目開き + 口開き |
| D | 目閉じ + 口閉じ |
| E | 目閉じ + 口半開き |
| F | 目閉じ + 口開き |
| 目眉マスク | 目と眉毛を Inpaint する範囲 |
| 口マスク | 口だけを Inpaint する範囲 |
| A 高品質版 | A 25 枚を Image2Image で整えたもの |

---

## 5. Creator Tools ワークフロー

### 5.1 入力

ユーザーはまず、以下の 1 枚だけを用意する。

```text
input/main.png
```

条件:

- 上半身キャラ
- 目は開いている
- 口は閉じている
- 背景は透明、または chroma key で抜ける単色背景
- 5×5 の方向差分に展開しやすい正面寄り画像

### 5.2 CLI 前処理

Phase 3 の CLI は、最初から 150 枚を作らない。
まず A 状態の 25 枚だけを作る。

```powershell
python tools/gotuber_creator.py build-a \
  --input input/main.png \
  --character my-character \
  --output output/my-character
```

処理:

1. 入力検証
2. 背景透過（必要な場合）
3. アップスケール
4. 4500×4500 へ整形
5. 5×5 に切り出し
6. `A/r0c0.webp` 〜 `A/r4c4.webp` を出力

出力:

```text
output/my-character/
  A/
    r0c0.webp
    r0c1.webp
    ...
    r4c4.webp
```

### 5.3 マスク生成

次に、A の 25 枚それぞれに対してマスクを生成する。

```powershell
python tools/gotuber_creator.py masks \
  --input output/my-character/A \
  --output output/my-character/masks
```

生成するマスク:

```text
output/my-character/masks/
  eyes_brows/
    r0c0.png
    ...
    r4c4.png
  mouth/
    r0c0.png
    ...
    r4c4.png
  review/
    r0c0_eyes_brows_review.png
    r0c0_mouth_review.png
    ...
```

マスク方針:

- **目と眉毛**は 1 レイヤー
- **口**は別レイヤー
- レビュー PNG は赤マスクで見えるようにする
- 目眉マスクと口マスクは重ならないことを基本にする
- ユーザーがペイントソフトで手修正しやすい単純 PNG にする

画像編集ソフト上では、以下のようなレイヤー構造を想定する。

```text
目と眉毛 マスク
口 マスク
r0c2 キャラクター
```

### 5.4 A 25 枚の高品質化

ユーザーは A の 25 枚を元に、ChatGPT / Codex / 画像生成 AI / Image2Image で高品質版を作る。

```text
A/r0c0.webp
  ↓ Image2Image
A_high/r0c0.png
```

狙い:

- 元絵の構図を維持
- 顔・髪・服の画風を整える
- 25 方向の破綻をユーザーがレビューできる状態にする

### 5.5 B〜F の Inpaint 生成

高品質 A 25 枚ができたら、それを元に B〜F を作る。

| 出力 | 使う元画像 | 使うマスク | AI 指示 |
|---|---|---|---|
| B | A 高品質版 | 口マスク | 口を少し開ける |
| C | A 高品質版 | 口マスク | 口を大きく開ける |
| D | A 高品質版 | 目眉マスク | 目を閉じる |
| E | D または A 高品質版 | 目眉 + 口マスク | 目閉じ + 口少し開け |
| F | D または A 高品質版 | 目眉 + 口マスク | 目閉じ + 口大きく開け |

最終出力:

```text
output/my-character/final/
  A/r0c0.webp ... r4c4.webp
  B/r0c0.webp ... r4c4.webp
  C/r0c0.webp ... r4c4.webp
  D/r0c0.webp ... r4c4.webp
  E/r0c0.webp ... r4c4.webp
  F/r0c0.webp ... r4c4.webp
```

---

## 6. CLI 予定

### 6.1 `build-a`

```powershell
python tools/gotuber_creator.py build-a --input input/main.png --character my-character --output output/my-character
```

A 25 枚を作る。

### 6.2 `masks`

```powershell
python tools/gotuber_creator.py masks --input output/my-character/A --output output/my-character/masks
```

目眉・口マスクを作る。

### 6.3 `validate`

```powershell
python tools/gotuber_creator.py validate --input output/my-character/final
```

最終 150 枚が GoTuber の読み込み仕様に合うか確認する。

### 6.4 `preview-manifest`

```powershell
python tools/gotuber_creator.py preview-manifest --input output/my-character
```

中間生成物と不足ファイルを一覧化する。

---

## 7. 実装フェーズ

### Phase 3.0: 仕様固定（このドキュメントが source of truth）

Phase 3.0 はコード実装ではなく、**仕様の固定**に焦点を当てる。
以下の仕様が確定したら Phase 3.1 以降の実装に進む。

#### 3.0.1 A 25 枚先行方式

GoTuber のランタイムは A-F × 5×5 = 150 枚を読むが、
Phase 3.0 は **A 状態（目開き + 口閉じ）の 25 枚だけを先に作る**。

理由:
- 初回ユーザーは「6 枚の表情シートを全部 AI で作る」負担が重い
- 1 枚のメイン画像から A 25 枚を作れば、GoTuber で最低限動作確認できる
- A 25 枚を高品質化してから B〜F を Inpaint で作る方が、画質の破綻が少ない

#### 3.0.2 出力ディレクトリ構造

```
output/my-character/
├── A/                          # Phase 3.1 で生成
│   ├── r0c0.webp
│   ├── r0c1.webp
│   ├── ...
│   └── r4c4.webp
├── masks/                      # Phase 3.2 で生成
│   ├── eyes_brows/             # 目と眉毛マスク（白=対象外、黒=対象）
│   │   ├── r0c0.png
│   │   ├── ...
│   │   └── r4c4.png
│   ├── mouth/                  # 口マスク（白=対象外、黒=対象）
│   │   ├── r0c0.png
│   │   ├── ...
│   │   └── r4c4.png
│   └── review/                 # 赤マスク PNG（ユーザー確認用）
│       ├── r0c0_eyes_brows_review.png
│       ├── r0c0_mouth_review.png
│       ├── ...
│       ├── r4c4_eyes_brows_review.png
│       └── r4c4_mouth_review.png
├── A_high/                     # ユーザーが作る A 高品質版（Phase 3.3 で使用）
│   ├── r0c0.png
│   ├── ...
│   └── r4c4.png
└── final/                      # 最終 150 枚（Phase 3.5 で使用）
    ├── A/
    │   └── r0c0.webp ... r4c4.webp
    ├── B/
    │   └── r0c0.webp ... r4c4.webp
    ├── C/
    │   └── r0c0.webp ... r4c4.webp
    ├── D/
    │   └── r0c0.webp ... r4c4.webp
    ├── E/
    │   └── r0c0.webp ... r4c4.webp
    └── F/
        └── r0c0.webp ... r4c4.webp
```

#### 3.0.3 マスク命名規則

| マスク種別 | ディレクトリ | ファイル名パターン | 用途 |
|---|---|---|---|
| 目眉マスク | `masks/eyes_brows/` | `r{row}c{col}.png` | 目と眉毛を Inpaint する範囲 |
| 口マスク | `masks/mouth/` | `r{row}c{col}.png` | 口だけを Inpaint する範囲 |
| 目眉レビュー | `masks/review/` | `r{row}c{col}_eyes_brows_review.png` | ユーザーが目眉マスクの範囲を確認 |
| 口レビュー | `masks/review/` | `r{row}c{col}_mouth_review.png` | ユーザーが口マスクの範囲を確認 |

マスク仕様:
- フォーマット: PNG (RGBA)
- 解像度: 各セル 1200×1200 px（既存 `_default` アセットと同じ。グリッドセルサイズ（アンカーピッチ）は 900 px）
- 色: **黒 (0,0,0) = マスク対象（Inpaint する範囲）**、**白 (255,255,255) = 保持する範囲**
- 目眉マスクと口マスクは **原則重ならないこと**。ただし `validate` では重複を検証しない（ユーザー手修正前提の推奨事項）
- ユーザーがペイントソフトで手修正しやすい単純な PNG にする

#### 3.0.4 レビュー PNG の命名規則と色ルール

レビュー PNG は、マスクの範囲を **赤の半透明オーバーレイ** で視覚化する。

| 項目 | 仕様 |
|---|---|
| ファイル名 | `r{row}c{col}_{mask_type}_review.png` |
| `mask_type` | `eyes_brows` または `mouth` |
| 赤色 | RGBA (255, 0, 0, 128) — 半透明赤 |
| 合成方法 | 元画像の上に赤マスクを重ねる |
| 用途 | ユーザーが「この範囲が Inpaint される」ことを確認 |

レビュー PNG の生成ロジック:
1. 元画像 `A/r{row}c{col}.webp` を読み込む
2. マスク画像 `masks/{type}/r{row}c{col}.png` を読み込む
3. マスクが黒のピクセルに赤 (255,0,0,128) をオーバーレイ
4. `masks/review/r{row}c{col}_{type}_review.png` として保存

#### 3.0.5 validate の最小要件

`validate` コマンドは最終的な 150 枚が GoTuber の読み込み仕様に合致するかを検証する。

検証項目（全てパスしたら OK）:

| # | 検証内容 | エラー例 |
|---|---|---|
| 1 | `final/` に A〜F の 6 ディレクトリが存在する | `final/D/ がない` |
| 2 | 各ディレクトリに `r{0-4}c{0-4}.webp` が 25 枚存在する | `A/r2c3.webp がない` |
| 3 | 各ファイルのサイズが 1200×1200 px | `A/r0c0.webp が 800×800` |
| 4 | 各ファイルが WebP フォーマット | `A/r0c0.png は WebP でない` |
| 5 | 各ファイルに透明 alpha チャネルが含まれる | `A/r0c0.webp が RGB のみ` |

出力:
- 全パス: `OK: 150 files validated` + exit 0
- 失敗: `ERROR: ...` + exit 1

#### 3.0.6 既存ランタイムとの互換性

Phase 3.0 で固定する仕様は、既存の GoTuber ランタイム（Phase 1/2）と **完全に互換** である:

- ファイル命名 `A/r{row}c{col}.webp` は `internal/character/atlas.go` が読む形式と同一
- ディレクトリ構造 `A-F/` は `config/default.yaml` の `sheets` 設定と整合
- WebP フォーマットは `golang.org/x/image/webp` でデコード可能
- 1200×1200 px は既存アセットと同じサイズ（グリッドセルサイズ 900 px はアンカーピッチ）

Phase 3.0 の仕様固定は **GoTuber 本体のコードを一切変更しない**。

#### 3.0.7 キャラクター名による出力ディレクトリ管理

Creator Tools CLI は `--character <name>` オプションでキャラクター名を受け取る。
出力ディレクトリは `output/<character-name>/` になる。

```powershell
python tools/gotuber_creator.py build-a \
  --input input/main.png \
  --character my-character \
  --output output/my-character
```

仕様:
- `--character` は省略可能。省略時は `--output` で指定したディレクトリをそのまま使う
- デフォルトキャラクター名は `_default`（既存ランタイムの `assets/characters/_default/` と整合）
- 将来 GoTuber メインアプリにキャラクター選択 UI/設定を追加する際の前提とする
- キャラクター切り替え自体は Phase 3 のスコープ外。**仕様固定のみ**

#### 3.0.8 サンプルキャラクターのライセンス

Phase 3 で提供するサンプルキャラクター画像（`assets/characters/_default/`）の利用条件:

- **動作検証・テスト利用**: 自由（GoTuber の動作確認用）
- **動画/配信利用**: yosia の明示許可が必要
- **再配布禁止**: サンプル画像をそのまま再配布しない
- **改変利用**: ユーザーが独自に改変した結果物の利用は自由（ただし元画像の再配布は不可）

> **注意**: サンプル画像は GoTuber の動作確認用。配信で使用する場合は yosia に許可を求めるか、ユーザー自身のキャラクター画像を用意すること。

### Phase 3.1: `build-a` CLI

- 既存 `tools/upscale_2x.py` を流用
- 既存 `tools/slice_character_sheets.py` の切り出しロジックを流用
- 1 枚入力から A 25 枚を出す

### Phase 3.2: マスク生成 CLI

- 目眉マスクの初期矩形生成
- 口マスクの初期矩形生成
- 赤マスク review PNG 生成
- ユーザーが手修正しやすい PNG 出力

### Phase 3.3: AI 作業用プロンプト生成

- A 高品質化用プロンプト
- B〜F Inpaint 用プロンプト
- 目眉マスク / 口マスクの使い分け説明

### Phase 3.4: `validate` CLI

- 150 枚存在確認
- `A-F/r{row}c{col}.webp` 命名確認
- サイズ確認
- 透明 alpha 確認

### Phase 3.5: ドキュメント統合

- `docs/新キャラ差し替え手順.md` を Creator Tools 前提へ更新
- `docs/01_画像生成用プロンプト.txt` と重複しないように整理
- 初回ユーザー向け quickstart 追加

### Phase 3.6: Depth Map Generator

Phase 3.6 は **Morph Renderer 用 depth map を事前生成する Creator Tools 拡張**とする。

ここでは depth map を **作るだけ**に留める。GoTuber runtime で depth map を読み込んでメッシュ変形する処理は Phase 4 に分離する。

目的:

- A〜F / r0c0〜r4c4 の各セル画像に対応する depth map を生成する
- 生成結果を `A/depth/r2c2.png` のように、各 sheet ディレクトリ配下へ保存する
- Phase 4 の Morph Renderer がそのまま読めるファイル配置を Phase 3 側で先に固定する
- depth map が無いキャラクターでも、既存 GoTuber runtime は従来通り動作できるようにする

非ゴール:

- runtime Morph Renderer の実装
- Ebitengine の `DrawTriangles` / mesh rendering
- αブレンドによるセル切り替え
- depth map を使った頂点変形
- runtime 中の深度推定 ML 実行

Phase 3.6 で生成する depth map は **オフライン生成物**であり、配信中の CPU/GPU 負荷を増やさない。

#### 3.6.1 出力配置

depth map は、対応する sheet ディレクトリの直下に `depth/` を作って保存する。

```text
assets/characters/{character-name}/
├── A/
│   ├── r0c0.webp
│   ├── r0c1.webp
│   ├── ...
│   ├── r4c4.webp
│   └── depth/
│       ├── r0c0.png
│       ├── r0c1.png
│       ├── ...
│       └── r4c4.png
├── B/
│   ├── r0c0.webp
│   └── depth/
│       └── r0c0.png ... r4c4.png
...
└── F/
    ├── r0c0.webp
    └── depth/
        └── r0c0.png ... r4c4.png
```

例:

```text
assets/characters/yosia_sample/A/depth/r2c2.png
```

この配置にする理由:

- `A/r2c2.webp` と `A/depth/r2c2.png` が 1 対 1 で対応して分かりやすい
- character pack を ZIP 配布する時に、画像と depth map が同じ sheet 配下にまとまる
- Phase 4 runtime が `image path -> depth path` を単純に変換できる

#### 3.6.2 Depth Map 仕様

| 項目 | 仕様 |
|---|---|
| フォーマット | PNG |
| 解像度 | 1200×1200 px |
| 色 | grayscale |
| 白 | 手前。頂点変形の影響を強く受ける |
| 黒 | 奥。頂点変形の影響を弱くする |
| 透明領域 | 可能なら元画像と同じ alpha を維持。難しい場合は grayscale RGB + alpha 255 でも可 |

depth map は character image と同じ `1200×1200 anchored` に揃える。

PNG の色形式は、初期実装では **grayscale** または **grayscale + alpha** を許容する。
`validate-depth` は「PNG として読み込めること」と「grayscale として扱えること」を確認する。
alpha がある場合は、Phase 4 runtime が透明領域の変形を弱めるための補助情報として使える。

理由:

- Phase 4 の mesh vertex が `SrcX/SrcY` で画像と depth map の同じ UV を参照できる
- 画像側だけ 1200×1200、depth 側だけ 900×900 だと、頂点変形時に座標変換が増える
- 既存 `_default` アセットのサイズと同じなので、runtime fallback を単純化できる

#### 3.6.3 CLI 予定

全 sheet をまとめて生成する場合:

```powershell
python tools/gotuber_creator.py generate-depth `
  --input assets/characters/yosia_sample `
  --sheets A,B,C,D,E,F
```

全 sheet を対象にする場合は、`--sheets` を省略してもよい。

```powershell
python tools/gotuber_creator.py generate-depth `
  --input assets/characters/yosia_sample
```

1 sheet だけ生成する場合:

```powershell
python tools/gotuber_creator.py generate-depth `
  --input assets/characters/yosia_sample `
  --sheets A
```

初期実装では、depth 推定モデルの選定を固定しすぎない。
まずは以下のどちらかを許容する。

- 自動推定: depth model / 外部 CLI / Python package を使って生成
- 手動配置: ユーザーが作った `depth/r{row}c{col}.png` を validate だけする

ただし、Phase 3.6 実装に入る前に **Phase 3.6.0: depth backend decision** を置く。
ここで以下を決めてから `generate-depth` を実装する。

- 既定 backend（例: Depth Anything V2 系、または同等の offline depth estimator）
- Windows でのセットアップ方法
- CPU/GPU どちらを初期サポートにするか
- `--backend auto/manual/<name>` の CLI 形
- backend が無い環境では `validate-depth` のみ使える fallback

#### 3.6.4 validate-depth

`validate-depth` は depth map が Phase 4 で読める形になっているかだけを確認する。

```powershell
python tools/gotuber_creator.py validate-depth `
  --input assets/characters/yosia_sample
```

検証項目:

| # | 検証内容 | エラー例 |
|---|---|---|
| 1 | 対象 sheet に `depth/` が存在する | `A/depth/ がない` |
| 2 | `r{0-4}c{0-4}.png` が 25 枚存在する | `A/depth/r2c3.png がない` |
| 3 | 各 depth map が 1200×1200 px | `A/depth/r0c0.png が 900×900` |
| 4 | PNG として読み込める | `A/depth/r0c0.png を decode できない` |
| 5 | grayscale として扱える | `A/depth/r0c0.png が想定外の色形式` |

depth map が存在しない場合、既存 GoTuber runtime はエラーにしない。
Phase 4 runtime でも depth map が無いセルは通常描画へ fallback する。

---

## 8. 完了基準 (DoD)

- [ ] 1 枚のメイン画像から A 25 枚を生成できる
- [ ] A 25 枚に対して目眉マスク / 口マスクを生成できる
- [ ] 赤マスク review PNG で範囲確認できる
- [ ] ユーザーが A 高品質版 → B〜F Inpaint へ進める手順が docs だけで分かる
- [ ] 最終 150 枚を `validate` で検証できる
- [ ] GoTuber 本体の character config と互換
- [ ] 既存 Phase 1 / Phase 2 runtime に影響しない

---

## 9. 想定工数

1〜2 週間

内訳:

- Phase 3.0 仕様固定: 0.5 日
- Phase 3.1 `build-a`: 1〜2 日
- Phase 3.2 マスク生成: 2〜3 日
- Phase 3.3 プロンプト生成: 1 日
- Phase 3.4 validate: 1 日
- Phase 3.5 docs 統合: 0.5〜1 日

---

## 10. 関連ドキュメント

- [PLAN.md](./PLAN.md)
- [PHASE1.md](./PHASE1.md)
- [PHASE2.md](./PHASE2.md)
- [PHASE4.md](./PHASE4.md) — Phase 3.6 で生成した depth map を runtime Morph Renderer で使う
- [post_release.md](./post_release.md)
- [新キャラ差し替え手順.md](./新キャラ差し替え手順.md)
