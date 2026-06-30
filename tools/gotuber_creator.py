#!/usr/bin/env python3
"""
gotuber_creator.py - GoTuber Creator Tools (Phase 3)

1 枚のメイン画像から GoTuber 用キャラクター素材を作る CLI ツール。

Phase 3.0 仕様固定済み。詳細は docs/PHASE3.md Section 7 "Phase 3.0: 仕様固定" 参照。

サブコマンド:
  build-a           1 枚入力から A 状態 25 枚を生成 (Phase 3.1)
  masks             A 25 枚から目眉/口マスクを生成 (Phase 3.2)
  validate          最終 150 枚が GoTuber 仕様に合致するか検証 (Phase 3.4)
  preview-manifest  中間生成物と不足ファイルを一覧化 (Phase 3.5)
  generate-depth    Morph Renderer 用 depth map を生成 (Phase 3.6)
  validate-depth    depth map が Phase 4 で読める形か検証 (Phase 3.6)

使い方:
  python tools/gotuber_creator.py build-a --input input/main.png --character my-character --output output/my-character
  python tools/gotuber_creator.py masks --input output/my-character/A --character my-character --output output/my-character/masks
  python tools/gotuber_creator.py validate --input output/my-character/final --character my-character
  python tools/gotuber_creator.py preview-manifest --input output/my-character --character my-character
  python tools/gotuber_creator.py generate-depth --input assets/characters/mychar --sheets A
  python tools/gotuber_creator.py generate-depth --input assets/characters/mychar --backend manual
  python tools/gotuber_creator.py validate-depth --input assets/characters/mychar

Phase 3.6 既定 backend: Depth Anything v3 (depth-anything-v3)。
手動配置は --backend manual で利用可能。
"""

from __future__ import annotations

import argparse
import math
import sys
from pathlib import Path

try:
    from PIL import Image, ImageFilter
except ImportError:
    Image = None  # type: ignore[assignment]
    ImageFilter = None  # type: ignore[assignment]


# ---------- 仕様定数 (Phase 3.0 で固定) ----------

# GoTuber ランタイム仕様
ROWS = 5
COLS = 5
SHEET_NAMES = ("A", "B", "C", "D", "E", "F")
CELL_SIZE = 900  # px
ATLAS_SIZE = ROWS * CELL_SIZE  # 4500 px

# マスク命名規則
MASK_EYES_BROWS = "eyes_brows"
MASK_MOUTH = "mouth"
MASK_TYPES = (MASK_EYES_BROWS, MASK_MOUTH)

# レビュー PNG 色 (半透明赤)
REVIEW_COLOR = (255, 0, 0, 128)

# デフォルトキャラクター名
DEFAULT_CHARACTER = "_default"

# Phase 3.6: Depth Map 仕様
DEPTH_DIR_NAME = "depth"
DEPTH_CELL_SIZE = 1200  # px — character image と同じ anchored size

# Depth Anything v3 既定モデル (HuggingFace Hub ID)
DA3_MODEL_ID = "depth-anything/DA3-LARGE-1.1"


def cell_filename(row: int, col: int, ext: str = "webp") -> str:
    """セルファイル名を生成 (例: r0c0.webp)。"""
    return f"r{row}c{col}.{ext}"


def mask_filename(row: int, col: int) -> str:
    """マスクファイル名を生成 (例: r0c0.png)。"""
    return f"r{row}c{col}.png"


def review_filename(row: int, col: int, mask_type: str) -> str:
    """レビュー PNG ファイル名を生成 (例: r0c0_eyes_brows_review.png)。"""
    return f"r{row}c{col}_{mask_type}_review.png"


# ---------- サブコマンド ----------

def cmd_build_a(args: argparse.Namespace) -> int:
    """1 枚入力から A 状態 25 枚を生成 (Phase 3.1 実装)。

    TODO Phase 3.1:
      1. 入力検証 (PNG/RGBA, 正方形推奨)
      2. 背景透過 (必要な場合、apply_green_key.py 流用)
      3. アップスケール (upscale_2x.py 流用)
      4. 4500×4500 へ整形
      5. 5×5 に切り出し
      6. A/r{row}c{col}.webp を出力
    """
    input_path = Path(args.input)
    output_dir = Path(args.output)

    if not input_path.is_file():
        print(f"ERROR: 入力ファイルが存在しない: {input_path}", file=sys.stderr)
        return 1

    print(f"Phase 3.1: build-a — 未実装")
    print(f"  入力: {input_path}")
    print(f"  出力: {output_dir}")
    print(f"  TODO: Phase 3.1 で実装")
    return 0


def cmd_masks(args: argparse.Namespace) -> int:
    """A 25 枚から目眉/口マスクを生成 (Phase 3.2 実装)。

    TODO Phase 3.2:
      1. A/r{row}c{col}.webp を 25 枚読み込む
      2. 各セルに対して目眉マスク矩形を生成
      3. 各セルに対して口マスク矩形を生成
      4. masks/eyes_brows/r{row}c{col}.png を出力
      5. masks/mouth/r{row}c{col}.png を出力
      6. masks/review/r{row}c{col}_{type}_review.png を出力
    """
    input_dir = Path(args.input)
    output_dir = Path(args.output)

    if not input_dir.is_dir():
        print(f"ERROR: 入力ディレクトリが存在しない: {input_dir}", file=sys.stderr)
        return 1

    print(f"Phase 3.2: masks — 未実装")
    print(f"  入力: {input_dir}")
    print(f"  出力: {output_dir}")
    print(f"  TODO: Phase 3.2 で実装")
    return 0


def cmd_validate(args: argparse.Namespace) -> int:
    """最終 150 枚が GoTuber 仕様に合致するか検証 (Phase 3.4 実装)。

    TODO Phase 3.4:
      1. final/ に A-F の 6 ディレクトリが存在するか
      2. 各ディレクトリに r{0-4}c{0-4}.webp が 25 枚存在するか
      3. 各ファイルのサイズが 900×900 px か
      4. 各ファイルが WebP フォーマットか
      5. 各ファイルに透明 alpha チャネルが含まれるか
    """
    input_dir = Path(args.input)

    if not input_dir.is_dir():
        print(f"ERROR: 入力ディレクトリが存在しない: {input_dir}", file=sys.stderr)
        return 1

    print(f"Phase 3.4: validate — 未実装")
    print(f"  入力: {input_dir}")
    print(f"  TODO: Phase 3.4 で実装")
    return 0


def cmd_preview_manifest(args: argparse.Namespace) -> int:
    """中間生成物と不足ファイルを一覧化 (Phase 3.5 実装)。

    TODO Phase 3.5:
      1. A/ が存在するか、25 枚あるか
      2. masks/ が存在するか、eyes_brows/ と mouth/ があるか
      3. masks/review/ が存在するか、50 枚あるか
      4. A_high/ が存在するか、25 枚あるか
      5. final/ が存在するか、150 枚あるか
      6. 不足ファイルを一覧表示
    """
    input_dir = Path(args.input)

    if not input_dir.is_dir():
        print(f"ERROR: 入力ディレクトリが存在しない: {input_dir}", file=sys.stderr)
        return 1

    print(f"Phase 3.5: preview-manifest — 未実装")
    print(f"  入力: {input_dir}")
    print(f"  TODO: Phase 3.5 で実装")
    return 0


# ---------- Phase 3.6: Depth Map Generator ----------


def _check_pillow() -> int:
    """Pillow が利用できない場合はエラーメッセージを出力して exit 1。"""
    if Image is None:
        print(
            "ERROR: Pillow がインストールされていません。",
            file=sys.stderr,
        )
        print(
            "  pip install Pillow でインストールしてください。",
            file=sys.stderr,
        )
        return 1
    return 0


def _validate_single_depth(path: Path, expected_size: tuple[int, int]) -> list[str]:
    """1 枚の depth map を検証し、エラーメッセージのリストを返す。

    検証項目 (docs/PHASE3.md 3.6.2):
      - PNG として読み込める
      - サイズが expected_size (1200×1200)
      - grayscale として扱える

    許容モード:
      - L (8-bit grayscale)
      - LA (8-bit grayscale + alpha)
      - I (32-bit integer grayscale)
      - F (32-bit float grayscale)
      - RGB/RGBA: R=G=B のみ (grayscale-equivalent)。任意色は拒否。
    """
    errors: list[str] = []
    try:
        img = Image.open(path)
    except Exception as exc:
        errors.append(f"{path.name}: PNG decode 失敗 ({exc})")
        return errors

    if img.size != expected_size:
        errors.append(
            f"{path.name}: サイズ {img.size}、期待値 {expected_size}"
        )

    mode = img.mode
    # Inherently grayscale modes — always valid
    if mode in {"L", "LA", "I", "F"}:
        img.close()
        return errors

    # RGB / RGBA: grayscale-equivalent (R=G=B) のみ許容
    if mode in {"RGB", "RGBA"}:
        # Red チャネルと Green チャネルの差分が 0 なら grayscale-equivalent
        r, g, b = img.split()[:3]
        from PIL import ImageChops

        diff_rg = ImageChops.difference(r, g)
        diff_rb = ImageChops.difference(r, b)
        # 全ピクセルで差分が 0 か確認 (最大値が 0 なら完全一致)
        max_diff_rg = max(diff_rg.getdata())
        max_diff_rb = max(diff_rb.getdata())
        if max_diff_rg > 0 or max_diff_rb > 0:
            errors.append(
                f"{path.name}: 色形式 '{mode}' は grayscale でない "
                f"(R≠G or R≠B。grayscale-equivalent は R=G=B が必要)"
            )
        img.close()
        return errors

    # その他のモード (P, CMYK, 1, etc.) は拒否
    errors.append(
        f"{path.name}: 想定外の色形式 '{mode}' "
        f"(許容: L, LA, I, F, または R=G=B の RGB/RGBA)"
    )
    img.close()
    return errors


def cmd_validate_depth(args: argparse.Namespace) -> int:
    """depth map が Phase 4 で読める形になっているか検証 (Phase 3.6)。

    検証項目 (docs/PHASE3.md 3.6.4):
      1. 対象 sheet に depth/ が存在する
      2. r{0-4}c{0-4}.png が 25 枚存在する
      3. 各 depth map が 1200×1200 px
      4. PNG として読み込める
      5. grayscale として扱える
    """
    rc = _check_pillow()
    if rc != 0:
        return rc

    input_dir = Path(args.input)
    if not input_dir.is_dir():
        print(f"ERROR: 入力ディレクトリが存在しない: {input_dir}", file=sys.stderr)
        return 1

    sheets = args.sheets.split(",") if args.sheets else list(SHEET_NAMES)
    expected_files = {cell_filename(r, c, "png") for r in range(ROWS) for c in range(COLS)}
    expected_size = (DEPTH_CELL_SIZE, DEPTH_CELL_SIZE)

    total_ok = 0
    total_errors: list[str] = []

    for sheet in sheets:
        depth_dir = input_dir / sheet / DEPTH_DIR_NAME
        if not depth_dir.is_dir():
            total_errors.append(f"{sheet}/{DEPTH_DIR_NAME}/ が存在しない")
            continue

        # 存在するファイルを確認 (r*.png だけでなく全ファイルをチェック)
        actual_files = {p.name for p in depth_dir.iterdir() if p.is_file()}
        missing = expected_files - actual_files
        extra = actual_files - expected_files

        for f in sorted(missing):
            total_errors.append(f"{sheet}/{DEPTH_DIR_NAME}/{f} がない")
        for f in sorted(extra):
            total_errors.append(f"{sheet}/{DEPTH_DIR_NAME}/{f} は想定外")

        # 各ファイルを検証
        for fname in sorted(expected_files & actual_files):
            fpath = depth_dir / fname
            errs = _validate_single_depth(fpath, expected_size)
            if errs:
                total_errors.extend(f"{sheet}/{DEPTH_DIR_NAME}/{e}" for e in errs)
            else:
                total_ok += 1

    if total_errors:
        for e in total_errors:
            print(f"ERROR: {e}", file=sys.stderr)
        print(
            f"\n{total_ok} files OK, {len(total_errors)} errors",
            file=sys.stderr,
        )
        return 1

    print(f"OK: {total_ok} depth maps validated ({len(sheets)} sheets)")
    return 0


# ---------- generate-depth バックエンド ----------


def _generate_depth_heuristic(
    src_path: Path, dst_path: Path
) -> list[str]:
    """Pillow のみで简单的な depth map を生成する (Phase 3.6 heuristic backend)。

    方式:
      1. 元画像を RGBA → L (grayscale) に変換
      2. エッジ検出 (FIND_EDGES) で輪郭を抽出
      3. 中心からの距離フィールドを加算 (人物は中心に近いほど手前と仮定)
      4. 0-255 に正規化して保存

    注意: これは ML ベースの深度推定ではない。Phase 4 のメッシュ変形が
    動作確認できる最小限の depth map を生成するためのヒューリスティック。
    高品質な depth map を使う場合は --backend manual で手動配置すること。
    """
    errors: list[str] = []
    try:
        img = Image.open(src_path)
    except Exception as exc:
        errors.append(f"読み込み失敗: {src_path.name} ({exc})")
        return errors

    # RGBA → L (grayscale) に変換
    gray = img.convert("L")
    img.close()
    w, h = gray.size

    # エッジ検出
    edges = gray.filter(ImageFilter.FIND_EDGES)

    # 中心からの距離フィールドを生成 (Pillow の ImageDraw で円形グラデーション)
    # 手前に近い = 白、奥に遠い = 黒
    from PIL import ImageDraw

    dist_field = Image.new("L", (w, h), 0)
    draw = ImageDraw.Draw(dist_field)
    cx, cy = w // 2, h // 2
    max_r = int(math.sqrt(cx * cx + cy * cy))

    # 外側から内側へ同心円で描画 (外=0, 内=255)
    steps = 64
    for i in range(steps):
        ratio = i / steps
        r = int(max_r * (1.0 - ratio))
        val = int(ratio * 255)
        draw.ellipse(
            [cx - r, cy - r, cx + r, cy + r],
            fill=val,
        )

    # エッジと距離を合成: 深度 = 0.6 * distance + 0.4 * edge
    # PIL のブレンドで高速化
    from PIL import ImageChops

    # edges を 0.4 倍にスケール
    edges_scaled = edges.point(lambda x: int(x * 0.4))
    dist_scaled = dist_field.point(lambda x: int(x * 0.6))

    result = ImageChops.add(dist_scaled, edges_scaled, scale=1, offset=0)

    # 1200×1200 にリサイズ (元画像サイズが異なる場合)
    if (w, h) != (DEPTH_CELL_SIZE, DEPTH_CELL_SIZE):
        result = result.resize(
            (DEPTH_CELL_SIZE, DEPTH_CELL_SIZE), Image.Resampling.LANCZOS
        )

    dst_path.parent.mkdir(parents=True, exist_ok=True)
    result.save(dst_path, "PNG")
    result.close()
    return errors


def _load_da3_model() -> tuple[object | None, list[str]]:
    """Depth Anything v3 モデルを 1 回だけ読み込み、(model, errors) を返す。

    成功時は model が返り、errors は空リスト。
    失敗時は model=None で errors にエラーメッセージが入る。
    """
    errors: list[str] = []

    try:
        import torch
    except ImportError:
        errors.append(
            "PyTorch がインストールされていません。"
            "pip install torch でインストールしてください。"
        )
        return None, errors

    try:
        from depth_anything_3.api import DepthAnything3
    except ImportError:
        errors.append(
            "depth-anything-3 がインストールされていません。"
            "pip install depth-anything-3 でインストールしてください。"
        )
        return None, errors

    device = torch.device("cuda" if torch.cuda.is_available() else "cpu")
    try:
        model = DepthAnything3.from_pretrained(DA3_MODEL_ID)
        model = model.to(device)
    except Exception as exc:
        errors.append(
            f"DA3 モデル読み込み失敗: {exc}\n"
            f"  モデル ID: {DA3_MODEL_ID}\n"
            "  ネットワークに接続して再実行するか、手動で weights を配置してください。"
        )
        return None, errors

    return model, errors


def _generate_depth_da3(
    model: object,
    src_path: Path,
    dst_path: Path,
) -> list[str]:
    """Depth Anything v3 で深度推定 depth map を生成する (Phase 3.6 DA3 backend)。

    方式:
      1. src_path を PIL Image で読み込む
      2. 事前読み込み済み DA3 モデルで inference を実行
      3. prediction.depth ([N,H,W] float32) を 0-255 grayscale PNG に正規化
      4. 1200×1200 にリサイズして dst_path に保存

    注意:
      - depth-anything-3 Python パッケージとモデル weights が必要
      - 初回実行時にモデルが自動ダウンロードされる場合がある
      - model は _load_da3_model() で事前読み込み済みであること
    """
    errors: list[str] = []
    import numpy as np
    from PIL import Image as PILImage

    # 画像読み込み
    try:
        img = PILImage.open(src_path).convert("RGB")
    except Exception as exc:
        errors.append(f"読み込み失敗: {src_path.name} ({exc})")
        return errors

    # DA3 inference
    try:
        prediction = model.inference([img])  # type: ignore[union-attr]
    except Exception as exc:
        errors.append(f"DA3 inference 失敗: {src_path.name} ({exc})")
        img.close()
        return errors

    # prediction.depth: [N,H,W] float32 — 最初の 1 枚を使用
    depth_np = prediction.depth[0]  # shape: (H, W), float32
    img.close()

    # float32 → uint8 に正規化 (0-255)
    d_min = float(depth_np.min())
    d_max = float(depth_np.max())
    if d_max - d_min < 1e-6:
        # 定数深度 → 全面 中間値
        depth_u8 = np.full_like(depth_np, 128, dtype=np.uint8)
    else:
        depth_norm = (depth_np - d_min) / (d_max - d_min)
        # 白=手前 / 黒=奥 に変換 (DA3 は通常 奥=大値 のため反転)
        depth_u8 = ((1.0 - depth_norm) * 255.0).clip(0, 255).astype(np.uint8)

    # NumPy array → PIL Image
    result = PILImage.fromarray(depth_u8, mode="L")

    # 1200×1200 にリサイズ (元画像サイズが異なる場合)
    if result.size != (DEPTH_CELL_SIZE, DEPTH_CELL_SIZE):
        result = result.resize(
            (DEPTH_CELL_SIZE, DEPTH_CELL_SIZE), PILImage.Resampling.LANCZOS
        )

    dst_path.parent.mkdir(parents=True, exist_ok=True)
    result.save(dst_path, "PNG")
    result.close()
    return errors


def cmd_generate_depth(args: argparse.Namespace) -> int:
    """Morph Renderer 用 depth map を生成 (Phase 3.6)。

    バックエンド:
      - depth-anything-v3: Depth Anything v3 で ML 深度推定 (既定)
      - manual: ユーザーが手動で depth map を配置する前提。既存ファイルを確認するだけ。
      - heuristic (debug-only): Pillow のエッジ検出+距離フィールド。非推奨。

    出力先: {input}/{sheet}/depth/r{row}c{col}.png
    """
    rc = _check_pillow()
    if rc != 0:
        return rc

    input_dir = Path(args.input)
    if not input_dir.is_dir():
        print(f"ERROR: 入力ディレクトリが存在しない: {input_dir}", file=sys.stderr)
        return 1

    sheets = args.sheets.split(",") if args.sheets else list(SHEET_NAMES)
    backend = args.backend
    ext = args.ext

    # DA3 バックエンドの場合、モデルを 1 回だけ読み込む
    da3_model = None
    if backend == "depth-anything-v3":
        da3_model, load_errors = _load_da3_model()
        if da3_model is None:
            for e in load_errors:
                print(f"ERROR: {e}", file=sys.stderr)
            return 1

    total_generated = 0
    total_skipped = 0
    total_errors: list[str] = []

    for sheet in sheets:
        sheet_dir = input_dir / sheet
        if not sheet_dir.is_dir():
            total_errors.append(f"{sheet}/ ディレクトリが存在しない")
            continue

        depth_dir = sheet_dir / DEPTH_DIR_NAME

        for r in range(ROWS):
            for c in range(COLS):
                src_name = cell_filename(r, c, ext)
                src_path = sheet_dir / src_name
                dst_path = depth_dir / cell_filename(r, c, "png")

                if not src_path.is_file():
                    total_errors.append(f"{sheet}/{src_name} が存在しない")
                    continue

                if backend == "manual":
                    # manual モード: 既存ファイルがあればスキップ、なければ警告
                    if dst_path.is_file():
                        total_skipped += 1
                    else:
                        total_errors.append(
                            f"{sheet}/{DEPTH_DIR_NAME}/{cell_filename(r, c, 'png')} "
                            f"がない (--backend manual のため手動配置が必要)"
                        )
                    continue

                if dst_path.is_file() and not args.force:
                    total_skipped += 1
                    continue

                # backend に応じた生成関数を選択
                if backend == "depth-anything-v3":
                    errs = _generate_depth_da3(da3_model, src_path, dst_path)
                elif backend == "heuristic":
                    errs = _generate_depth_heuristic(src_path, dst_path)
                else:
                    total_errors.append(f"不明な backend: {backend}")
                    continue

                if errs:
                    total_errors.extend(
                        f"{sheet}/{DEPTH_DIR_NAME}/{e}" for e in errs
                    )
                else:
                    total_generated += 1

    if total_errors:
        for e in total_errors:
            print(f"ERROR: {e}", file=sys.stderr)

    print(
        f"generate-depth ({backend}): "
        f"{total_generated} generated, {total_skipped} skipped, "
        f"{len(total_errors)} errors"
    )
    return 1 if total_errors else 0


# ---------- メイン ----------

def main() -> int:
    parser = argparse.ArgumentParser(
        description="GoTuber Creator Tools — 1 枚入力からキャラクター素材を作る",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__,
    )
    sub = parser.add_subparsers(dest="command", help="サブコマンド")

    # build-a
    p_build = sub.add_parser("build-a", help="1 枚入力から A 25 枚を生成")
    p_build.add_argument("--input", required=True, help="入力 PNG ファイル")
    p_build.add_argument("--character", default=DEFAULT_CHARACTER,
                         help=f"キャラクター名 (default: {DEFAULT_CHARACTER})")
    p_build.add_argument("--output", required=True, help="出力ディレクトリ")

    # masks
    p_masks = sub.add_parser("masks", help="A 25 枚から目眉/口マスクを生成")
    p_masks.add_argument("--input", required=True, help="A ディレクトリ")
    p_masks.add_argument("--character", default=DEFAULT_CHARACTER,
                         help=f"キャラクター名 (default: {DEFAULT_CHARACTER})")
    p_masks.add_argument("--output", required=True, help="マスク出力ディレクトリ")

    # validate
    p_validate = sub.add_parser("validate", help="最終 150 枚を検証")
    p_validate.add_argument("--input", required=True, help="final ディレクトリ")
    p_validate.add_argument("--character", default=DEFAULT_CHARACTER,
                            help=f"キャラクター名 (default: {DEFAULT_CHARACTER})")

    # preview-manifest
    p_manifest = sub.add_parser("preview-manifest", help="中間生成物を一覧化")
    p_manifest.add_argument("--input", required=True, help="キャラクターディレクトリ")
    p_manifest.add_argument("--character", default=DEFAULT_CHARACTER,
                            help=f"キャラクター名 (default: {DEFAULT_CHARACTER})")

    # validate-depth (Phase 3.6)
    p_vdepth = sub.add_parser(
        "validate-depth",
        help="depth map が Phase 4 で読める形か検証",
    )
    p_vdepth.add_argument(
        "--input", required=True,
        help="キャラクターディレクトリ (例: assets/characters/mychar)",
    )
    p_vdepth.add_argument(
        "--sheets", default=None,
        help="検証対象シート (カンマ区切り。省略時は A-F 全 sheet)",
    )

    # generate-depth (Phase 3.6)
    p_gdepth = sub.add_parser(
        "generate-depth",
        help="Morph Renderer 用 depth map を生成",
    )
    p_gdepth.add_argument(
        "--input", required=True,
        help="キャラクターディレクトリ (例: assets/characters/mychar)",
    )
    p_gdepth.add_argument(
        "--sheets", default=None,
        help="生成対象シート (カンマ区切り。省略時は A-F 全 sheet)",
    )
    p_gdepth.add_argument(
        "--backend", default="depth-anything-v3",
        choices=["depth-anything-v3", "manual", "heuristic"],
        help=(
            "depth 生成バックエンド。"
            "depth-anything-v3=Depth Anything v3 ML 深度推定 (既定)。"
            "manual=手動配置済みファイルの確認のみ。"
            "heuristic=Pillow エッジ検出+距離フィールド (debug-only、非推奨)"
        ),
    )
    p_gdepth.add_argument(
        "--ext", default="webp",
        help="入力画像の拡張子 (default: webp)",
    )
    p_gdepth.add_argument(
        "--force", action="store_true",
        help="既存 depth map があっても上書きして再生成",
    )

    args = parser.parse_args()

    if args.command is None:
        parser.print_help()
        return 1

    commands = {
        "build-a": cmd_build_a,
        "masks": cmd_masks,
        "validate": cmd_validate,
        "preview-manifest": cmd_preview_manifest,
        "validate-depth": cmd_validate_depth,
        "generate-depth": cmd_generate_depth,
    }

    return commands[args.command](args)


if __name__ == "__main__":
    sys.exit(main())
