# GoTuber — Phase 4: Morph Renderer 詳細設計

> **ステータス**: 未着手（Phase 3.6 depth map 生成後に開始）
> **最終更新**: 2026-06-30
> **親プラン**: [PLAN.md](./PLAN.md) v0.4.8

---

## 1. 目標

Phase 4 は **Morph Renderer** とする。

目的は、既存の 5×5×6 画像構造を維持したまま、セル切り替えの「パチッ」とした見え方を減らし、PNG キャラクターをより滑らかに動いているように見せること。

GoTuber は Live2D / 3D rig を目指さない。Phase 4 では、以下の軽量な組み合わせで見た目を改善する。

1. セル切り替え時の αブレンド
2. 画像を小さな mesh に分割して描く Morph Renderer
3. Phase 3.6 で生成した depth map による depth-weighted elastic deformation

人間語で言うと、今の GoTuber は「パラパラ漫画」に近い。Phase 4 では、向きが変わる時に同じ画像を少しプリンのようにゆっくり曲げ、別セルへ移る時だけ αブレンドで滑らかにつなぐ。

---

## 2. 非ゴール

- Live2D 互換 rig の実装
- VRM / 3D モデル対応
- runtime 中の depth 推定 ML 実行
- 顔パーツ単位の高精度物理 simulation
- 外部 VTuber アプリへの motion 送信
- Phase 3 Creator Tools の画像生成フロー変更

Phase 4 は **描画層の改善**であり、キャラクター素材の基本仕様 `A-F/r{row}c{col}.webp` は変えない。

---

## 3. 入力ファイル構造

Phase 4 は、Phase 1/2 で使っている通常の character image と、Phase 3.6 で生成する depth map を読む。

```text
assets/characters/{character-name}/
├── A/
│   ├── r0c0.webp
│   ├── ...
│   ├── r4c4.webp
│   └── depth/
│       ├── r0c0.png
│       ├── ...
│       └── r4c4.png
├── B/
│   └── depth/
│       └── r0c0.png ... r4c4.png
...
└── F/
    └── depth/
        └── r0c0.png ... r4c4.png
```

例:

```text
A/r2c2.webp
A/depth/r2c2.png
```

depth map が存在しない場合は、Morph Renderer を無効化して従来の `DrawImage()` 相当へ fallback する。

---

## 4. Runtime アーキテクチャ

Phase 4 は既存 `internal/game/game.go` の描画処理を置き換えるのではなく、描画専用 component を追加する。

想定構成:

```text
internal/game/game.go
  └── currentCell(), sheetForState()
        ↓
internal/render/morph_renderer.go
  ├── cell transition alpha blend
  ├── mesh generation
  ├── depth-weighted vertex offset
  └── fallback normal draw

internal/character/atlas.go
  ├── character image cache
  └── depth image lookup（Phase 4 で追加検討）
```

責務分離:

| 場所 | 責務 |
|---|---|
| `game.go` | どの sheet / row / col を表示するか決める |
| `morph_renderer.go` | その画像をどう描くか決める |
| `atlas.go` | 画像と depth map を読み込む |

この分け方にすると、Morph Renderer が不安定でも通常描画 fallback で配信中の安全性を維持できる。

---

## 5. Phase 4.0: Cell Transition αブレンド

最初に実装するのは、セル切り替え時の αブレンド。

現在の描画は、`currentCell()` が変わった瞬間に表示画像も即座に変わる。

```text
old: A/r2c2.webp
new: A/r2c3.webp
```

Phase 4.0 では、一定フレームだけ old と new を重ねる。

```text
frame 0: old alpha 1.0 / new alpha 0.0
frame 1: old alpha 0.8 / new alpha 0.2
frame 2: old alpha 0.6 / new alpha 0.4
...
final:   old alpha 0.0 / new alpha 1.0
```

実装方針:

- 前回描画した `sheet,row,col` を保持する
- 今回の `sheet,row,col` と違う場合だけ transition を開始する
- transition duration の初期値は **100ms** とする
- Tweaks UI では将来 80〜120ms 程度の範囲で調整可能にする
- `DrawImageOptions.ColorScale` の alpha を使って 2 枚重ねる
- transition 中にさらにセルが変わった場合は、現在表示中の new cell を起点に再開始する

制約:

- mouth / blink の高速切り替えまで全部長く blend すると、口パクがぼやける可能性がある
- 初期実装では head direction cell の切り替えを優先し、mouth/blink は短め transition または blend 無効も検討する

DoD:

- セル切り替え時のパチッ感が軽減される
- FPS 低下が体感できない
- depth map が無いキャラクターでも動く
- transition 無効化フラグを用意できる

---

## 6. Phase 4.1: Mesh Renderer

次に、1 枚画像をそのまま `DrawImage()` するのではなく、複数の小さい四角形 mesh として描く。

想定 mesh:

```text
32×32 grid
vertices: 33×33
triangles: 32×32×2
```

各 vertex は以下を持つ。

| 値 | 意味 |
|---|---|
| `SrcX/SrcY` | 元画像上の UV / pixel 座標 |
| `DstX/DstY` | 画面上の描画座標 |
| `ColorA` | αブレンド用 alpha |

Ebitengine では `DrawTriangles` を使う。

```text
image texture + vertices + indices
  ↓
screen.DrawTriangles(...)
```

初期実装では、mesh 変形なしで `DrawImage()` と同じ見た目になることを確認する。

段階:

1. `DrawImage()` と同じ位置・スケールで mesh 描画する
2. 画像の端が欠けないことを確認する
3. `FilterLinear` 相当の見た目を維持する
4. 既存 UI / Tweaks / camera mode へ影響しないことを確認する

DoD:

- mesh rendering ON/OFF で見た目がほぼ一致する
- 1200×1200 sprite を 1280×720 window に縮小しても欠けない
- `DrawImage()` fallback を残す
- depth map が無い状態でも mesh renderer 単体で動く

---

## 7. Phase 4.2: Depth-weighted Elastic Morph

Phase 4.2 で、Phase 3.6 の depth map を使って mesh 頂点を少し動かす。

考え方:

```text
camera yaw / pitch
  ↓
target deformation
  ↓
spring smoothing
  ↓
depth map で頂点ごとの強さを変える
  ↓
mesh vertex DstX/DstY をずらす
```

depth の意味:

- 白に近い部分: 手前。よく動く
- 黒に近い部分: 奥。あまり動かない

基本式のイメージ:

```text
depth = depthMap.Sample(u, v)  // 0.0〜1.0
offsetX = visualYaw   * depth * strengthX * weight(u, v)
offsetY = visualPitch * depth * strengthY * weight(u, v)
```

`weight(u,v)` は、画面全体が平行移動して見えないようにするための追加重み。

初期案:

- 左右輪郭は yaw の影響を強める
- 上側の髪・頭頂部は pitch / elastic sway の影響を少し強める
- 胴体中心は動きを抑える
- 顔の中央は動かしすぎない

Elastic smoothing:

```text
visualYaw += (targetYaw - visualYaw) * 0.15
velocity += (target - current) * spring
velocity *= damping
```

最初は単純 EMA でよい。揺れ感が足りなければ spring + damping へ進む。

DoD:

- 正面静止時にキャラが勝手に歪み続けない
- 左右に向いた時、髪や輪郭が少し遅れて動く
- 大きい向き変更では 5×5 cell 切り替え + αブレンドが優先される
- depth map が無いセルは通常 mesh / DrawImage fallback になる
- 低スペック環境で FPS が落ちすぎない

---

## 8. 実装フェーズ

| Phase | 内容 | 状態 |
|---|---|---|
| 4.0 | Cell Transition αブレンド | 未着手 |
| 4.1 | Mesh Renderer | 未着手 |
| 4.2 | Depth-weighted Elastic Morph | 未着手 |
| 4.3 | Tweaks UI 追加（Morph ON/OFF、強度、transition duration） | 未着手 |
| 4.4 | Performance tuning / fallback | 未着手 |

Phase 4.0 と 4.1 は depth map なしでも実装できる。
Phase 4.2 だけ Phase 3.6 の depth map generator に依存する。

---

## 9. リスク & 対策

| リスク | 影響 | 対策 |
|---|---|---|
| mesh 頂点数が多すぎて FPS が落ちる | 中 | 初期は 16×16 または 32×32。Tweaks で品質を下げられるようにする |
| depth map 品質が悪くて変形が気持ち悪くなる | 中 | depth morph は OFF 可能。通常描画 fallback を必ず残す |
| mouth / blink の切り替えが αブレンドでぼやける | 中 | head direction と expression sheet で transition duration を分ける |
| 変形で画像端が欠ける | 中 | mesh 外周に padding / clamp を入れる。1200×1200 anchored の余白を活かす |
| Live2D っぽい複雑 rig へ肥大化する | 中 | Phase 4 は描画層の軽量 deformation まで。パーツ分解 rig は非ゴールに固定 |

---

## 10. 完了基準 (DoD)

- [ ] αブレンドでセル切り替えの違和感が減る
- [ ] mesh renderer ON/OFF で通常描画との見た目差分が小さい
- [ ] depth map があるセルだけ elastic morph できる
- [ ] depth map が無いセルは従来描画へ fallback する
- [ ] Morph ON/OFF を Tweaks から切り替えられる
- [ ] 1280×720 window で FPS 低下が体感できない
- [ ] Phase 1 / Phase 2 の mouth sync、blink、camera tracking に regression がない

---

## 11. 関連ドキュメント

- [PLAN.md](./PLAN.md)
- [PHASE1.md](./PHASE1.md)
- [PHASE2.md](./PHASE2.md)
- [PHASE3.md](./PHASE3.md) — Phase 3.6 Depth Map Generator
