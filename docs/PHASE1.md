# GoTuber — Phase 1: Pure Go PNGTuber MVP 詳細設計

> **ステータス**: Q's 決定済（Go 1.25 確認待ち）
> **最終更新**: 2026-06-15
> **親プラン**: [PLAN.md](./PLAN.md) v0.4.3

---

## 1. 目標

`tomari-guruguru`（React/Vite/JSX のブラウザアプリ）の **Golang 完全書き換え** + **メインマイクで同時 Realtime 口パク**（メインの VTuber 使用ケース）。

前身と同等の機能（5×5 角度 × 6 状態、150 フレームのマウス追従・口パク・まばたき）を pure Go 単一バイナリで実現する。

> **Q6 反映**: 音声ファイル口パクは **Phase 1.5+ で再評価**。Phase 1 はマイクのみに集中。

---

## 2. ゴール / 非ゴール

### 2.1 ゴール

- 同じ PNG 形式（5×5×6 = 150 枚）がそのまま使える
- 同じマウス追従・口パク・まばたきの挙動
- 同じ Tweaks 概念
- 同じスライス画像生成 Python ツール同梱
- 単一バイナリ配布（15 MB 以下）
- 透過背景 + クリックスルー
- 24/7 連続運用
- kill switch（Esc / SIGINT）
- **メインマイクで同時 Realtime 口パク**（VTuber 配信用途の主機能）

### 2.2 非ゴール

- カメラ入力 → Phase 2（**保留中**、Q8 で再評価）
- 音声ファイル口パク → Phase 1.5+ 検討枠（Q6 保留）
- VMC Protocol 出力 → Phase 3
- Live2D / 3D モデル対応
- アセット同梱（権利上不可）

---

## 3. 設計の核

**malgo 完結オーディオ（Phase 1 スコープ）**: malgo 1 個で「マイク入力 + RMS 解析」を統一。**Phase 1 はマイク入力のみ**（ファイル再生・スピーカー出力は Phase 1.5+ で同アーキテクチャを拡張）。Ebitengine `audio/{mp3,wav,vorbis}` は **Phase 1.5+ で利用**（PCM デコーダーとして、Player/Context は起動しない方針は維持、PLAN.md §0.5 参照）。

**5×5 で全方向カバー**: 5 行（pitch: 上→下）× 5 列（yaw: 左→右）= 25 セルで頭部の向きを表現。

**6 状態（A〜F）**: 目（開け/閉じ）× 口（とじ/中間/開け）の組み合わせで表情を表現。

詳細アーキテクチャは [PLAN.md §4](./PLAN.md#4-アーキテクチャ) 参照。

---

## 4. 実装項目

### Phase 1.1: プロジェクト雛形

- `go mod init github.com/<owner>/GoTuber` で Go module 作成
- 依存導入: `Ebitengine v2.9.9` / `malgo v0.11.25` / `ebitenui v0.7.3` / `golang.org/x/image v0.42.0` / `gopkg.in/yaml.v3`
- 最小 `main.go`: 空の Ebitengine ウィンドウ
- SIGINT ハンドラ + Esc キーリッスン（kill switch）
- ディレクトリ構造（`cmd/`, `internal/`, `assets/`, `config/`, `tools/`, `scripts/`, `docs/`）
- ビルドスクリプト雛形（`build.ps1` / `build.sh`）
- テスト: `internal/killswitch/signal_test.go` — `Esc` 押下 / SIGINT で終了

### Phase 1.2: 透過ウィンドウ + クリックスルー

- **必ず `ebiten.SetWindowMousePassthrough(true)` を `Game.Update()` の初回呼び出し内で実行**（Issue #3222 対策）
- `ebiten.SetWindowFloating(true)` 併用
- フォーカスフリッカー検証: KASOU (Debian X11) で xdotool を使い「クリックスルー有効化直後 200ms のキー入力ロスト」計測
- 問題があれば 60 フレーム遅延発火オプション追加
- フォールバック: Win32 `WS_EX_TRANSPARENT` を `golang.org/x/sys/windows` から直接操作

### Phase 1.3: スプライトアトラス loader

- `init()` で `image/png`（標準）+ `golang.org/x/image/webp` を `image.RegisterFormat` 登録
- **起動戦略**: ウィンドウ表示 → "Loading…" → `atlas.ready bool` 完了後にアバター表示
- **展開方式**: よく使う 1 シート（25 枚）+ 近傍予備のみプリロード、残りは **遅延デコード**（[PLAN.md §8.1](./PLAN.md#81-メモリ予算) 参照）
- 並列デコード（goroutine fan-out、`errgroup`）
- クラッシュ対策: 展開完了フラグで Update 描画スキップ

### Phase 1.4: 設定 YAML + 起動時バリデーション

- `internal/character/config.go` で YAML 読み込み
- フェイルファスト:
  - `base_path` を `filepath.Abs` + `filepath.Clean` で解決
  - 6 つのシートディレクトリ（A〜F）存在確認
  - 各シートに 25 枚画像存在確認
  - `ext` が "webp" または "png"
- 失敗時: `MessageBox` (Win) / `zenity` (Linux) / `osascript` (mac) でエラー → `exit 1`

### Phase 1.5: マウス追従

- `internal/mouse/follow.go`
- `smoothing` → `responsiveness` 改名（元 `smoothing` 値が小さいほど追従が遅い = 意味が逆だったため）
- ロジック: target {x, y} を [-1, 1] で clamp、current に lerp で収束
- 5×5 セルに cell {r, c} をマップ
- テスト: `internal/mouse/follow_test.go`
  - `TestFollow_TargetClamp`: target が [-1, 1] に clamp
  - `TestFollow_SmoothingConverges`: smoothing k で current が target に収束

### Phase 1.6: 自動まばたき

- `internal/blink/scheduler.go`
- 確率分布（二度瞬き 22% / ゆっくり 6% / 通常 72%）
- 不規則な間隔（通常 1.8〜4.5s、たまに 0.7〜1.5s、ぼーっと 4.5〜9s）
- テスト: `internal/blink/scheduler_test.go`
  - `TestBlink_Distribution`: 1 万回サンプリングで 22/6/72 ± 2% に収まる

### Phase 1.7: malgo マイク + エンベロープ + 口パク

- **malgo 完結オーディオ（Phase 1 スコープ）**
- malgo Capture モード（**入力のみ**。Duplex は Phase 1.5+ でファイル再生時に必要）
- 入力形式: `malgo.FormatS16`、mono、48 kHz
- エンベロープフォロワー: attack 0.5 / release 0.05（立ち上がり速、減衰遅。自然な音声挙動）
- 口パク閾値: 0.05 (closed→half) / 0.20 (half→open)（int16 [-1, 1] RMS 較正）
- ヒステリシス: **±0.02 RMS 値デッドゾーン**（時間ベースではない）
  - MouthClosed → MouthHalf: envelope > 0.07
  - MouthHalf → MouthClosed: envelope < 0.03
  - MouthHalf → MouthOpen:   envelope > 0.22
  - MouthOpen → MouthHalf:   envelope < 0.18
  - **Open→Closed 直接遷移なし**（必ず Half 経由。soft landing 設計）
- **ファイル再生（mp3/wav/ogg）口パクは Phase 1.5+ で追加**（Q6 保留、ユーザー判断）
- スレッド: audio スレッド (malgo callback) → `atomic.StoreUint64` → game スレッド → `atomic.LoadUint64`
- フェイルセーフ: `audio.NewMover()` 失敗時（デバイスなし等）は mover=nil で続行、口パク無効
- テスト: `internal/audio/envelope_test.go`（Phase 1.10 で追加）
  - `TestEnvelope_AttackRelease`: attack 0.5 / release 0.05
  - `TestMouth_Hysteresis`: closed↔half, half↔open の閾値検証
  - `TestRMS_Normalization`: int16 → [0, 1] 変換

### Phase 1.8: Tweaks パネル + CJK フォント

- `internal/tweaks/panel.go`（ebitenui ベース）
- CJK フォント埋め込み（Noto Sans CJK JP、`go:embed`）
- **Phase 1 は英語ラベル + 日本語フォントのみ**。i18n は需要が出たら Phase 2 以降で `gotext`

### Phase 1.9: ビルドスクリプト

- `build.ps1`（Windows native）
- `build.sh`（WSL Ubuntu / Linux バイナリ）
- `dev.ps1` / `dev.sh`（開発ループ）
- `tools/requirements.txt` 同梱（Pillow, numpy のバージョン固定）
- `tools/LICENSE-third-party` 同梱（依存ライブラリのライセンス一覧）
- README に `pip install -r tools/requirements.txt` 手順明記

### Phase 1.10: 仕上げ

- README.md
- LICENSE（MIT）
- `docs/PHASE1.md`（実装ログ）
- **`go test ./...` 全パス**確認

---

## 5. 完了基準 (DoD)

- [ ] WSL Ubuntu で `./gotuber` が 1 秒以内に起動する
- [ ] **起動時に "Loading…" テキストが表示され、`atlas.ready = true` 後にアバターへ遷移する（クラッシュなし）**
- [ ] KASOU (Debian 13) にデプロイして 60 FPS 安定
- [ ] マウス追従が滑らか（25 方向すべて遷移）
- [ ] **メインのマイクで同時 Realtime 口パクが反応**（F32 [-1,1] 較正、`thHalf`/`thFull` 感度調整可）
- [ ] 自動まばたきが不規則に発火（二度・ゆっくり含む）
- [ ] 透過背景で OBS 配信に重ねられる
- [ ] クリックスルー有効時、配下アプリのクリックを遮らない（OBS で確認）
- [ ] **フォーカスフリッカー検証 OK**（xdotool 計測）
- [ ] `Esc` 押下で即座に終了する（kill switch）
- [ ] ビルド成果物が **15 MB** 以下
- [ ] **`go test ./...` 全パス**
- [ ] README にビルド手順・KASOU デプロイ手順が明記

> **削除された DoD**（Q6 反映）: 「音声ファイルで口パクが反応（mp3/wav/ogg）」— Phase 1.5+ で再評価

---

## 6. 想定工数

1〜2 週間（実働 40〜60 h）

内訳:
- Phase 1.1（雛形 + kill switch）: 0.5 日
- Phase 1.2（透過 + クリックスルー）: 0.5〜1 日
- Phase 1.3（アトラス）: 1〜2 日
- Phase 1.4（設定 + バリデーション）: 0.5 日
- Phase 1.5（マウス追従）: 0.5〜1 日
- Phase 1.6（まばたき）: 0.5 日
- Phase 1.7（malgo マイク + エンベロープ + 口パク）: **1〜2 日**（Q6 反映でファイル再生削除）
- Phase 1.8（Tweaks + フォント）: 1〜2 日
- Phase 1.9（ビルドスクリプト）: 0.5 日
- Phase 1.10（仕上げ + テスト）: 0.5〜1 日

---

## 7. 親プランとのクロスリファレンス

| 項目 | 親プラン参照 |
|---|---|
| §0.5 設計判断ログ（malgo 完結、pure Go 採用理由） | [PLAN.md §0.5](./PLAN.md#05-設計判断ログ) |
| §2.1 スタック選定（Ebitengine, malgo, webp, ebitenui） | [PLAN.md §2.1](./PLAN.md#2-スタック選定) |
| §3 機能マッピング（元 React → Go 移植マップ） | [PLAN.md §3](./PLAN.md#3-機能マッピング元--go) |
| §4 アーキテクチャ（ディレクトリ、データフロー、コア型） | [PLAN.md §4](./PLAN.md#4-アーキテクチャ) |
| §4.2.1 パフォーマンス予算 | [PLAN.md §4.2.1](./PLAN.md#421-パフォーマンス予算) |
| §4.4 設定ファイル（YAML 構造） | [PLAN.md §4.4](./PLAN.md#4-アーキテクチャ) |
| §4.5 起動時バリデーション | [PLAN.md §4.5](./PLAN.md#4-アーキテクチャ) |
| §6 ビルド & 開発 | [PLAN.md §6](./PLAN.md#6-ビルド--開発) |
| §7 リスク & 対策 | [PLAN.md §7](./PLAN.md#7-リスク--対策) |
| §8.1 メモリ予算 | [PLAN.md §8.1](./PLAN.md#81-メモリ予算) |
| §11 未解決事項（**Q1, Q3, Q4, Q12 確定**、Q6 / Q8 保留） | [PLAN.md §11](./PLAN.md#11-未解決事項実装前確認) |

---

## 8. 関連ドキュメント

- 元プロジェクト: `../tomari-guruguru/README.md`
- 元マウス追従: `../tomari-guruguru/src/app.jsx`
- 元口パク: `../tomari-guruguru/src/talk-app.jsx`
- 元まばたき: `../tomari-guruguru/src/app.jsx`（74〜110 行目）
- Ebitengine Issue #3222: https://github.com/hajimehoshi/ebiten/issues/3222
