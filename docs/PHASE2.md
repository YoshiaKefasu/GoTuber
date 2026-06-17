# GoTuber — Phase 2: Camera 入力 詳細設計

> **ステータス**: 保留中（2026-06-15 ユーザー判断、Q8 で再評価待ち）
> **最終更新**: 2026-06-15
> **親プラン**: [PLAN.md](./PLAN.md) v0.4.3

**保留理由**: Phase 1 を「マウス追従のみ」に集中させる方針確定（Q8）。Phase 1 リリース後に顔モデル選定を再評価し、Phase 2 着手可否を判断する。

---

## 1. 目標

Webcam で **顔追従・口パク・まばたき** を自動化し、配信者がマウスから解放される完全カメラ VTuber 体験を実現する。

---

## 2. ゴール / 非ゴール

### 2.1 ゴール

- 顔を左右上下に動かすとキャラが同じ方向を見る（yaw / pitch → 5×5 セル）
- 口を開けるとキャラの口が動く
- 目を閉じるとキャラが目をつむる
- カメラが消えても自動的にマイク/マウスモードにフォールバック
- 30 FPS 以上を維持（顔検出は別 goroutine）
- `-tags gocv` なしビルドが引き続き成功（Phase 1 の軽量性が維持される）
- 顔を左右に向ける = 体を左右に向けるため、5×5 yaw 軸で **2D 体左右も同時サポート**（新フォーマット不要）

### 2.2 非ゴール（Phase 2.5+ 検討枠）

- 体の傾き（lean）検出
- 腕の動き
- 全身 Live2D 風
- 3D モデル対応
- 複数カメラ対応

---

## 3. 設計の核：5×5 で 2D 体左右をカバー

**顔を左右に向ける = 体を左右に向ける**（普通は一致）ので、5×5 (c0..c4) の yaw 軸がそのまま「**2D 体左右**」になる。

| ユーザー動作 | 5×5 セル | キャラ表示 |
|---|---|---|
| 頭を左に向ける | c0 | 左向き |
| 頭を右に向ける | c4 | 右向き |
| 頭を上に向ける | r0 | 上目遣い |
| 頭を下に向ける | r4 | 下目遣い |

**新フォーマット不要、新モデル不要、新アセット不要**。Phase 2.4 のマッピングだけで対応。

カバーできないのは **body lean / 腕の動き / 足の動き** のみ。Phase 2.5+ で別軸追加（必要時）。

---

## 4. 実装項目

### Phase 2.0: OpenCV 4.13.0 インストール

- `scripts/setup-gocv.ps1` (Windows) と `scripts/setup-gocv.sh` (WSL) 作成
- WSL: `apt install libopencv-dev` (4.13 系列)
- Windows: gocv の自動ダウンロード + vcvarsall 設定
- README に `scripts/setup-gocv.sh` / `.ps1` をリンク
- 動作確認スクリプト同梱（カメラ 0 番で顔検出できるか）

### Phase 2.1: gocv 依存追加

- `go.mod` に `gocv.io/x/gocv v0.43.0` 追加
- `internal/camera/` 配下を `//go:build gocv` でガード
- **Phase 1 ビルドには影響しない**（`-tags gocv` なしビルド）
- ビルド検証: `go build ./...` と `go build -tags gocv ./...` の両方が通る

### Phase 2.2: Webcam デバイス列挙 + 選択 UI

- gocv で `/dev/video0` (Linux) / 0 番 (Windows) を列挙
- Tweaks パネルに「カメラデバイス選択」ドロップダウン追加
- デバイスラベル（OS から取得可能なら使用、不可能なら連番）

### Phase 2.3: 顔検出

- **第一選択**: MediaPipe Face Mesh（ONNX 経由、468 ランドマーク、頭部姿勢込み）
- **代替**: YuNet（5 ランドマーク）+ `solvePnP` で頭部姿勢推定
- 入力: グレースケール化 + リサイズ（モデル推奨サイズに）
- 出力: ランドマーク + 頭部姿勢（yaw, pitch, roll）
- 顔未検出時の挙動: 1 秒以上未検出 → audio モードへ自動切替

### Phase 2.4: 顔中心 → mouse follow target へのマッピング

- 顔中心の正規化座標 (-1, 1) をそのまま cell {r, c} にマップ
- 5×5 の c0..c4 (yaw 軸) = 「2D 体左右」を兼ねる
- cell の補間は mouse follow の lerp ロジックと共有
- テスト: `internal/camera/mapper_test.go`
  - `TestMapper_FaceCenterToCell`: 顔中心 (-1, 0) → c0
  - `TestMapper_SmoothingConverges`: 検出遅延がある状態でも追従が滑らか

### Phase 2.5: 口の縦横比（MAR）→ mouth state

- 上下唇 landmark の距離 / 唇幅 = MAR (Mouth Aspect Ratio)
- 閾値ベースで mouth 状態を判定（Closed / Half / Open）
- 音声マイクより優先するか、ブレンドするかは実装時に決定
- 案: 「カメラ MAR > 閾値 なら camera、そうでなければ audio」のフォールバック
- 案: 「両者の最大値」のブレンド

### Phase 2.6: 目のアスペクト比（EAR）→ blink state

- 目の縦幅 / 横幅 = EAR (Eye Aspect Ratio)
- EAR < 閾値 → blink = true
- 二重瞬き / ゆっくり瞬きは audio モードの scheduler と共有（カメラ検出が優先）
- テスト: `internal/camera/blink_test.go`

### Phase 2.7: メインループとは別 goroutine

- 顔検出は 15〜50 ms かかるので、メインの 16.67 ms 予算に収まらない
- 別 goroutine で実行し、channel で最新結果を main loop に通知
- channel バッファは 1（最新値で上書き、古いフレーム drop）
- main loop は最新値を読み取って avatar.State に反映
- 顔検出 goroutine は最後の 1 フレームだけ保持、無限バッファ化しない

### Phase 2.8: ミラー反転オプション

- カメラ映像を左右反転（鏡像）
- 人間の自然なカメラフィード感覚
- 設定ファイルで ON/OFF（`config/default.yaml` に `camera.mirror: true` 追加）

### Phase 2.9: フェイルセーフ

- カメラ権限拒否 → error 表示 + audio/mouse モード継続
- デバイス未接続 → error 表示 + audio/mouse モード継続
- gocv クラッシュ → ウィンドウの X ボタンで安全停止 + ログ（Phase 1.14 で仕様確定）
- 1 秒以上顔未検出 → audio モードへ自動切替
- 顔の bounding box がカメラフレームの 5% 未満（顔が遠い）→ audio モード

---

## 5. 完了基準 (DoD)

- [ ] Webcam 0 番デバイスで顔が検出され追従する
- [ ] 顔が左右に動くとキャラが振向く（yaw → 列方向）
- [ ] 顔が上下に動くとキャラが上目遣い/下目遣い（pitch → 行方向）
- [ ] 口を開けると 1 フレーム以内に口パク反映
- [ ] 目を閉じるとキャラが目をつむる
- [ ] 顔を画面外に出すと 1 秒以内に audio モードにフォールバック
- [ ] 30 FPS 以上を維持（顔検出は別 goroutine）
- [ ] `-tags gocv` なしビルドが引き続き成功
- [ ] `scripts/setup-gocv.sh` / `.ps1` が README からリンクされ動作
- [ ] カメラ切断時にクラッシュせず audio/mouse モードへ移行
- [ ] `go test ./... -tags gocv -v -race` 全パス

---

## 6. 想定工数

3〜5 週間

内訳（参考）:
- 環境構築 (2.0〜2.1): 0.5〜1 週
- 顔検出実装 (2.2〜2.3): 1〜2 週
- マッピング (2.4〜2.6): 0.5〜1 週
- 並行化 (2.7): 0.5 週
- 仕上げ (2.8〜2.9): 0.5 週

---

## 7. 親プランとのクロスリファレンス

| 項目 | 親プラン参照 |
|---|---|
| Q8（保留、Phase 2 着手時に再評価） | [PLAN.md §11 Q8](./PLAN.md#11-未解決事項実装前確認) |
| R2 (OpenCV インストール摩擦) | [PLAN.md §7 R2](./PLAN.md#7-リスク--対策) |
| R6 (バイナリサイズ増) | [PLAN.md §7 R6](./PLAN.md#7-リスク--対策) |
| 4.2.1 パフォーマンス予算 (Phase 2 行) | [PLAN.md §4.2.1](./PLAN.md#421-パフォーマンス予算) |
| 8.1 メモリ予算 | [PLAN.md §8.1](./PLAN.md#81-メモリ予算) |
| 8.2 エラー UX (実行時致命的) | [PLAN.md §8.2](./PLAN.md#82-エラー-ux) |
| Phase 1 アーキテクチャの受け継ぎ | [PLAN.md §4](./PLAN.md#4-アーキテクチャ) |

---

## 8. 将来の検討（Phase 2.5+）

Phase 2 完了後、必要に応じて独立フェーズで切り出す:

- **体の傾き（lean）**: 顔向きに依存しない独立軸。実装には body landmark 検出が必要
- **腕の動き**: MediaPipe Body (33 landmarks) または MoveNet (17 keypoints) が必要
- **全身 Live2D 風**: アセットパイプライン再設計が必要、Phase 1〜3 との互換性ゼロ
- **複数カメラ**: 顔 + 全身の 2 カメラ運用
- **ハンドトラッキング**: 指の動き検出、特殊用途向け

これらは **Phase 2.5 / Phase 4+** として別フェーズで検討。今は Phase 1 着手優先。
