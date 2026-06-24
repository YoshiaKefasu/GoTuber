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

使い方:
  python tools/gotuber_creator.py build-a --input input/main.png --character my-character --output output/my-character
  python tools/gotuber_creator.py masks --input output/my-character/A --character my-character --output output/my-character/masks
  python tools/gotuber_creator.py validate --input output/my-character/final --character my-character
  python tools/gotuber_creator.py preview-manifest --input output/my-character --character my-character
"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path


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

    args = parser.parse_args()

    if args.command is None:
        parser.print_help()
        return 1

    commands = {
        "build-a": cmd_build_a,
        "masks": cmd_masks,
        "validate": cmd_validate,
        "preview-manifest": cmd_preview_manifest,
    }

    return commands[args.command](args)


if __name__ == "__main__":
    sys.exit(main())
