# Post-Release TODO

最終リリース後に着手する改善・拡張タスクの一覧。
優先度: 🔴 高 / 🟡 中 / 🟢 低

## 🔴 高優先度: ワークフロー自動化

### 1. キャラ生成ワークフローの 1 コマンド化（上級者向け / Phase 3 後）

**目的**: ユーザー (配信者) が AI で生成した 6 枚 (A〜F) の green screen PNG を、**chromakey 透過 → 2x x 2 アップスケール (4500×4500) → 5×5 スライス** を **1 コマンドで** 透過済 150 枚 WebP に変換し、`assets/characters/_default/` まで配置する。

**位置づけ**: Phase 3 の Creator Tools は「1 枚入力 → A 25 枚 → マスク → AI 補完」を初回ユーザー向けの本道にする。この TODO は、すでに A〜F 6 枚を用意できる上級者向けのフル自動パイプラインとして post-release に残す。

**現在のワークフロー** (yosia さんが手作業で実行した 6 ステップ):

1. `green_input/` に `*#00FF00.png` を 6 枚配置
2. `python tools/apply_green_key.py green_input/ green_alpha/` (chroma key 透過)
3. リネーム `A_open_close.png` 〜 `F_close_open.png` (手作業 PowerShell)
4. `python tools/upscale_2x.py 新キャラ資料_4500/ 新キャラ資料_4500_out/ --target 4500` (**2x x 2 = 4x アップスケール** → 4500×4500 リサイズ)
5. `python tools/slice_character_sheets.py --source 新キャラ資料_4500_out/ --slices-out 出力先/` (5×5 = 25 枚にスライス、計 150 枚 WebP)
6. 出力 150 枚を `assets/characters/_default/` に移動 (手作業)

**目標**: 上記 6 ステップを **`build_default_character.py <入力フォルダ>` 1 コマンドで** 全自動実行。

**統合パイプライン**:

| 段階 | 処理 | 既存スクリプト | 統合先での扱い |
|------|------|---------------|---------------|
| 1 | 入力検証 (`*#00FF00.png` 6 枚) | - | 自動マッピング |
| 2 | **chroma key 透過** (`apply_green_key.py`) | 単体スクリプト | import 呼び出し |
| 3 | リネーム (A/B/C/D/E/F 接頭辞) | 手作業 | 自動 |
| 4 | **2x x 2 アップスケール + 4500×4500 リサイズ** (`upscale_2x.py`) | 単体スクリプト | subprocess |
| 5 | **5×5 スライス → 150 枚 WebP** (`slice_character_sheets.py`) | 単体スクリプト | subprocess |
| 6 | `assets/characters/_default/` にデプロイ | 手作業 | 自動 (`--auto-deploy` フラグ) |

**CLI**:

```bash
# 標準実行 (6 枚入力 → 中間フォルダ生成 → 150 枚 WebP 出力)
python tools/build_default_character.py <入力フォルダ>

# 自動デプロイ (出力 150 枚を assets/characters/_default/ に上書き)
python tools/build_default_character.py <入力フォルダ> --auto-deploy

# ドライラン (実行せずパイプライン確認のみ)
python tools/build_default_character.py <入力フォルダ> --dry-run

# オプション
python tools/build_default_character.py <入力フォルダ> \
    --threshold 80 \            # chroma key 閾値 (デフォルト 80)
    --target 4500 \             # アップスケール後サイズ
    --scale 2 \                 # 1 段階の倍率 (2x x 2 = 4x)
    --upscale-model <model> \   # カスタム NCNN モデル
    --device-id 0 \             # GPU index
    --keep-intermediate         # 中間ファイル保持 (デバッグ用)
```

**入力ファイル名規則** (自動シートマッピング):

| シート | ファイル名パターン | 例 |
|--------|------------------|-----|
| A (目開け + 口とじ) | `メイン（目を開いた）#00FF00.png` | yosia さんの実例 |
| B (目開け + 口中間) | `小さく「あ」の（目を開いた）#00FF00.png` | |
| C (目開け + 口開け) | `大きく「あ」の（目を開いた）#00FF00.png` | |
| D (目閉じ + 口とじ) | `メイン（目を瞑った）#00FF00.png` | |
| E (目閉じ + 口中間) | `小さく「あ」の（目を瞑った）#00FF00.png` | |
| F (目閉じ + 口開け) | `大きく「あ」の（目を瞑った）#00FF00.png` | |

**DoD**:

- 1 コマンドで **6 枚 green screen PNG → 透過済 150 枚 WebP → `_default/` まで完走**
  - 途中の chroma key + 2x x 2 アップスケール + 5×5 スライスも自動
- `--dry-run` モードで事前確認可能
- chroma key 閾値、GPU 選択等のオプションが CLI から指定可能
- 既存の単体スクリプト (apply_green_key / upscale_2x / slice_character_sheets) のインターフェースに変更なし
- `docs/新キャラ差し替え手順.md` から `build_default_character.py` への参照を追加
- パイプライン全体の所要時間をログ出力 (各段階の所要時間含む)

**工数**: 1-2 時間

### 1.5. 音声ファイル再生 (Q6 / Phase 1.5+)

**背景**: Phase 1 はマイク入力のみ。音楽ファイル (.mp3/.wav) や音声ファイルを再生してキャラクターの口をパクパクさせる機能 (リップシンク動画作成用途) は、Q6 で「リアルタイムマイクと同時使用しないため優先度低い」と判断し Phase 1.5+ に保留していた。公開リリースまでにユーザーニーズがあれば復活。

**実装範囲**:

- Ebitengine v2 の `audio` パッケージ (`mp3` / `wav` / `vorbis` デコーダー) を初期化
- ファイル選択 UI (Tweaks パネルに "Play Audio File" ボタン + ネイティブダイアログ)
- 既存 `Mover` (RMS + Envelope + MouthTracker) を audio ファイル再生にも流用
- Phase 1.7 の malgo と同時に動作させる (e.g. マイク OFF + ファイル ON でリップシンク動画作成)

**非採用案**:

- Ebitengine audio + malgo 出力の共存: 同一プロセス内 2 つの audio context は実装複雑
- ffmpeg 経由: 外部依存追加

**DoD**:

- 任意の mp3/wav を読み込んでキャラクターが口に反映
- Tweaks で再生 / 一時停止 / ファイル選択
- malgo マイクと排他利用 (両方 ON は不可)

**工数**: 4-6 時間

### 1.6. UI 改善: 左上設定ボタン常駐 + Ctrl+Shift+H で設定ボタンのみ表示/非表示 (ユーザー要望 2026-06-17)

**背景**: yosia さんから「キャラクターの左上に設定ボタン UI を置いて、それを Ctrl+Shift+H で表示/非表示できるようにしたい」という UI 改善要望あり。F1 は既存挙動のまま**残す** (両立)。Phase 1 では優先度低と判断して保留。

**現状 (Phase 1)**:
- 起動直後: キャラクターのみ (UI ゼロ)
- F1 キー: Tweaks パネル オン/オフ
- Ctrl+Shift+H: Tweaks パネルを含む**全 UI** を強制非表示

**目標 (post_release)**:
- キャラクター左上に設定ボタン (⚙ アイコン) を**デフォルト常駐表示** (F1 不要)
- 設定ボタンクリックで Tweaks パネルが開く
- F1 キー: 引き続き Tweaks パネル オン/オフ (既存挙動を維持、**F1 とボタンは両立**)
- Ctrl+Shift+H: **設定ボタンのみ** 表示/非表示トグル (Tweaks パネルは触らない)
  - 表示に戻すのも同じ Ctrl+Shift+H
  - 配信時に設定ボタンのみ隠せて、キャラは触らず、F1 でいつでも Tweaks 開ける

**実装メモ**:

- 設定ボタン位置: キャラクター左上 (Ebitengine `image.DrawImage` + 透明背景 + 矩形 hit test)
- ボタンの ebitenui 化 or 素朴な ebiten 描画: シンプル優先なら後者
- クリック判定: `ebiten.CursorPosition()` + ボタン矩形 hit test
- Ctrl+Shift+H の挙動変更: 現在は全 UI 強制非表示 → 設定ボタンのみにスコープ縮小
- 既存の F1 は無改修、既存の Tweaks パネル無改修
- uiHidden フラグの用途を変更: 「Tweaks パネル非表示」ではなく「設定ボタンのみ非表示」に

**DoD**:

- 起動直後からキャラクター左上に設定ボタンが表示される
- 設定ボタンをクリックで Tweaks パネル表示
- F1 キーでも Tweaks パネル表示 (既存挙動維持、ボタンと F1 は**両立**)
- Ctrl+Shift+H で**設定ボタンのみ** 表示/非表示トグル
- もう一度 Ctrl+Shift+H で設定ボタン再表示
- Tweaks パネル自体は Ctrl+Shift+H で消えない (F1 で制御)

**工数**: 2-3 時間

---

## 🟡 中優先度

### 2. GoTuber Character Format v1

**目的**: GoTuber 用キャラクター素材の標準フォーマットを明文化し、Creator Tools / 手作業 / AI 生成のどの経路でも同じ構造へ着地できるようにする。

**背景**: 現在の runtime は `A-F` × `5x5` の 150 枚を読む。Phase 3 Creator Tools では、まず A 25 枚を作り、マスクと AI 補完で B〜F を作る。将来の拡張表情や配布用 character pack を扱うには、キャラ素材の標準フォーマットを先に固める必要がある。

**想定フォーマット**:

```text
character/
  character.yaml
  A/r0c0.webp ... r4c4.webp
  B/r0c0.webp ... r4c4.webp
  C/r0c0.webp ... r4c4.webp
  D/r0c0.webp ... r4c4.webp
  E/r0c0.webp ... r4c4.webp
  F/r0c0.webp ... r4c4.webp
  masks/
    eyes_brows/r0c0.png ... r4c4.png
    mouth/r0c0.png ... r4c4.png
  metadata/
    prompts.md
    source_notes.md
```

**DoD**:

- `character.yaml` v1 の必須項目 / 任意項目を定義
- A〜F 150 枚、マスク、metadata の配置規約を定義
- `tools/gotuber_creator.py validate` がこの形式を検証できる
- README / 新キャラ差し替え手順から参照できる

**工数**: 0.5〜1 日

### 3. Expression Trigger System

**目的**: `>.<` などの追加表情セットを、MediaPipe の口開き・瞬き・顔向き・ホットキーなどで切り替えられるようにする。

**背景**: Phase 2 で MediaPipe から顔向き・瞬きが取れるようになった。将来的には「口を大きく開けたら `>.<`」「強く目を閉じたら `ぎゅっ`」のようなリアクション表情を GoTuber 内で完結させたい。

**想定ルール例**:

```yaml
expressions:
  x_x:
    basePath: expressions/x_x
    trigger:
      cameraMouthOpen: high
      minDurationMs: 120
      holdMs: 800
```

**トリガー候補**:

- `cameraMouthOpen`: MediaPipe の mouth ratio / MAR
- `eyeClosed`: EAR blink filter
- `headPitch` / `headYaw`: 顔向き
- hotkey
- manual button in Tweaks

**DoD**:

- 拡張表情セットの配置規約を定義
- trigger YAML の最小仕様を定義
- 通常 A〜F 表情と追加表情の優先順位を定義
- `>.<` 表情セットをサンプル仕様として docs 化

**工数**: 1〜2 日

### 4. chroma key 閾値の改善

**問題**: 現状の `apply_green_key.py` の閾値 80 では、AI 生成画像の**髪や服の周辺の微妙な緑 (RGB ~90-120)** が透過されない。

**対応**:

- 閾値 80 → 60 程度まで下げて再テスト
- 閾値を下げるとキャラの緑系パーツ (例: 緑色の服、緑の目) が巻き込まれるリスクあり
- HSV 色空間での色相ベース判定に切り替える (option: `--use-hsv`)
- AI 再生成時にプロンプトで「鮮明な緑背景 (saturated pure green)」を指示する対策も検討

**DoD**:

- 髪や服の周辺に緑色の線が残らない
- キャラ本体 (緑系パーツを除く) が巻き込まれない

**工数**: 30 分

### 5. ドキュメント整合性

**問題**: キャラ作成手順が複数ファイルに分散している。

- `docs/01_画像生成用プロンプト.txt` — AI プロンプト
- `docs/新キャラ差し替え手順.md` — スライス手順
- `docs/01_画像生成用プロンプト.txt` Section [6] — アップスケール手順
- `tools/build_default_character.py` の docstring — ワークフロー概要 (TODO)

**対応**:

- `docs/新キャラ差し替え手順.md` を「AI 生成 → 1 コマンド → 完了」の最新フローに書き換え
- `tools/build_default_character.py` ヘルプを全手順の単一情報源にする
- `docs/01_画像生成用プロンプト.txt` から重複手順 (Section [6] の個別スクリプト) を削除 or 簡略化

**DoD**: 新規ユーザーが 3 ファイル読む必要なく、1 ファイルの手順で完結

**工数**: 30 分

### 6. AI 再生成時の背景純白 / 純緑強制プロンプト

**問題**: AI が「グレー背景」と言っても実際は (137, 139, 149) のような**微妙に色味のあるグレー**を生成。chroma key や 4 隅サンプル方式でも完全除去困難。

**対応**:

- AI プロンプトに「単色 #00FF00 純緑 (saturated pure green)」を強調
- Stable Diffusion 系の ControlNet で「全画面緑」を強制する手法の調査
- もしくは: 「純白 (#FFFFFF) 背景」+ 白 key の二段構えスクリプト

**DoD**: 6 枚再生成時に背景がほぼ完全単色 (RGB 標準偏差 < 5) で chroma key 不要な状態

**工数**: AI 再生成テスト含む 1-2 時間

---

## 🟢 低優先度 (改善)

### 7. 元 tomari-guruguru データの復元

**問題**: 作業中、`assets/characters/_default/` の 150 枚 (元 tomari 由来) を誤って全削除。`A:\Temp\opencode\_default_backup_*` 退避は 0 枚だった (私の手順バグ)。

**対応**:

- yosia さんが `D:\GitHub\GoTuber_ws\tomari-guruguru\public\slices2/` から再コピー (必要なら)
- ライセンス的に問題 (再配布禁止) なので `_default/` ではなく `_reference/` 等の名前で退避
- 開発時の参考用として保持

**DoD**: 開発時に元データへのアクセス手段がある

**工数**: 10 分 (コピーだけ)

### 8. Phase 1.10 視覚テスト

**問題**: Phase 1.10 完了後も実機起動テスト未実施。yosia さんの `bin/gotuber.exe` 起動で初めて全機能の動作確認予定。

**対応**:

- yosia さんの Windows で `bin/gotuber.exe` 実行
- 確認項目:
  - 透過背景 (黒以外の部分だけ表示)
  - クリックスルー (キャラクター背後をクリックできる)
  - 5×5 マウス追従 (キャラクターがマウス方向に振り向く)
  - 自動まばたき (D/E/F 切替)
  - メインマイク口パク (A/B/C 切替)
  - F1 で Tweaks パネル
  - ~~Esc / Q で終了~~ → Phase 1.14 で削除予定、ウィンドウの X ボタン (全 OS graceful) または Ctrl+C (Unix: graceful / Windows: 即終了) で閉じる
  - OBS ウィンドウキャプチャで透過表示される

**DoD**: 全機能 OK 確認 → Phase 1 完全完了宣言

**工数**: 10 分 (yosia さんの手動テスト)

### 9. スライスツールのテスト追加

**問題**: `tools/slice_character_sheets.py` は yosia さんから MIT 継承した 648 行のスクリプト。テスト未追加。

**対応**:

- `tools/slice_character_sheets_test.py` を作成
- 最低限のスモークテスト: 5×5 グリッドが正しく切り出されるか
- コンポーネントモード / リサイズグレー除去のテスト

**DoD**: CI で全テスト pass

**工数**: 1-2 時間 (テスト作成)

### 10. DirectML パスの復活 (オプション)

**問題**: `tools/upscale_2x.py` の DirectML パスを試したが、RTX 2060 で `D3D12_ERROR_REMOVED_DEVICE` (887A0020) が出て CPU フォールバックした。

**対応**:

- DirectML 1.13+ で Windows 11 23H2 だと動く可能性あり
- もしくは `torch-directml` で PyTorch 経由の DirectML
- 優先度低 (NCNN Vulkan で実用速度出てるので)

**DoD**: DirectML パスが RTX 2060 で安定動作

**工数**: 2-3 時間

---

## 既知の制限 (現テストデータ)

### A. 髪や服周辺の緑残り

- chroma key 閾値 80 では AI 生成画像の**微妙な緑 (RGB 90-120)** が残ることがある
- 視覚的に目立たないが、厳密な透明処理が必要なら閾値調整 or AI 再生成

### B. キャラクターの輪郭周辺の影

- AI がキャラの周辺に「影」を描き込むことがある
- 4 隅サンプル方式 (`remove_gray_background.py`) や chroma key では完全除去困難
- テスト用なら許容範囲

### C. キャラの一貫性

- AI で 6 表情を別々に生成すると、**髪型・目・服装が微妙にずれる** 可能性
- 解決法: Photoshop / GIMP / Krita で目と口の部分だけトリミングして組み合わせる (元プロンプト P122-123 推奨)
- もしくは ComfyUI / A1111 の ControlNet + IP-Adapter で一貫性確保

---

## 進捗

- [ ] 1. ワークフロー自動化 (`tools/build_default_character.py`、上級者向け / Phase 3 後)
- [ ] 2. GoTuber Character Format v1
- [ ] 3. Expression Trigger System
- [ ] 4. chroma key 閾値改善
- [ ] 5. ドキュメント整合性
- [ ] 6. AI 再生成プロンプト改善
- [ ] 7. 元データ復元 (必要なら)
- [ ] 8. Phase 1.10 視覚テスト
- [ ] 9. スライスツールテスト追加
- [ ] 10. DirectML パス復活

最終更新: 2026-06-24
