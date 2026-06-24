# GoTuber — Phase 3: Creator Tools 詳細設計

> **ステータス**: 未着手（Phase 2 完了後に着手）
> **最終更新**: 2026-06-24
> **親プラン**: [PLAN.md](./PLAN.md) v0.4.7

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
python tools/gotuber_creator.py build-a --input input/main.png --output output/my-character
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

### Phase 3.0: 仕様固定

- A 25 枚先行方式を仕様化
- `output/my-character/` の中間ディレクトリ構造を固定
- マスク命名規則を固定
- レビュー PNG の色（赤）を固定

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
- [post_release.md](./post_release.md)
- [新キャラ差し替え手順.md](./新キャラ差し替え手順.md)
