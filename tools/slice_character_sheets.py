#!/usr/bin/env python3
"""
slice_character_sheets.py - キャラクター sprite sheet を 25 枚に分割する。

GoTuber のキャラクターアセットは 6 シート (A-F) × 5×5 セル = 150 枚の
個別 PNG として配置する必要がある:
    assets/characters/<name>/A/r{0-4}c{0-4}.png
    assets/characters/<name>/B/...
    ...
    assets/characters/<name>/F/...

このツールは 5×5 が並んだ 1 枚のスプライトシートから 25 枚の個別 PNG を
生成する。元 tomari-guruguru 由来の設計 (MIT 継承) を踏襲。

Usage:
    # 単一シート (5×5 500x500 想定)
    python slice_character_sheets.py --input sheet_A.png --output assets/characters/my_char/A

    # 6 シート一括 (カンマ区切り)
    python slice_character_sheets.py \\
        --input-sheet A:sheet_A.png,B:sheet_B.png,C:sheet_C.png,D:sheet_D.png,E:sheet_E.png,F:sheet_F.png \\
        --output-dir assets/characters/my_char

    # WebP 入力も対応 (Pillow が対応形式)
    python slice_character_sheets.py --input sheet.webp --output ./out --cell-size 200

Requirements:
    pip install -r tools/requirements.txt  # Pillow
"""
import argparse
import sys
from pathlib import Path

from PIL import Image


def slice_sheet(
    input_path: Path,
    output_dir: Path,
    rows: int = 5,
    cols: int = 5,
    cell_size: int = 100,
    overwrite: bool = False,
) -> int:
    """
    1 枚のスプライトシートを rows × cols 個の個別 PNG に分割する。

    戻り値: 実際に書き出したファイル数。

    セル座標系:
      row 0 がシート上端、col 0 が左端。
      出力ファイル名: r{row}c{col}.png
    """
    if not input_path.exists():
        print(f"ERROR: input not found: {input_path}", file=sys.stderr)
        return 0

    img = Image.open(input_path)
    expected_size = (cols * cell_size, rows * cell_size)
    if img.size != expected_size:
        print(
            f"WARNING: {input_path} is {img.size}, expected {expected_size}. "
            f"余白はクロップされます。",
            file=sys.stderr,
        )

    output_dir.mkdir(parents=True, exist_ok=True)

    count = 0
    for r in range(rows):
        for c in range(cols):
            # シートから該当セルを切り出し
            left = c * cell_size
            top = r * cell_size
            right = left + cell_size
            bottom = top + cell_size
            cell = img.crop((left, top, right, bottom))

            out_path = output_dir / f"r{r}c{c}.png"
            if out_path.exists() and not overwrite:
                continue
            cell.save(out_path, "PNG")
            count += 1

    return count


def parse_sheet_pairs(spec: str) -> list[tuple[str, Path]]:
    """
    "A:path1,B:path2" 形式を [("A", Path("path1")), ("B", Path("path2"))] に変換。
    """
    pairs = []
    for chunk in spec.split(","):
        chunk = chunk.strip()
        if not chunk:
            continue
        if ":" not in chunk:
            raise ValueError(f"invalid --input-sheet entry (expected SHEET:PATH): {chunk!r}")
        sheet, path_str = chunk.split(":", 1)
        sheet = sheet.strip().upper()
        path = Path(path_str.strip())
        if sheet not in "ABCDEF":
            raise ValueError(f"invalid sheet name (expected A-F): {sheet!r}")
        pairs.append((sheet, path))
    return pairs


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        description="キャラクター sprite sheet (5×5) を 25 枚の個別 PNG に分割する",
    )
    group = parser.add_mutually_exclusive_group(required=True)
    group.add_argument(
        "--input", type=Path,
        help="入力スプライトシート (単一ファイルモード)",
    )
    group.add_argument(
        "--input-sheet", type=str,
        help="複数シート一括モード: 'A:path1,B:path2,...' 形式",
    )
    parser.add_argument(
        "--output", type=Path,
        help="単一ファイルモードの出力ディレクトリ",
    )
    parser.add_argument(
        "--output-dir", type=Path,
        help="複数シートモードの出力ベースディレクトリ (<output-dir>/A/, <output-dir>/B/ ...)",
    )
    parser.add_argument("--rows", type=int, default=5, help="行数 (default: 5)")
    parser.add_argument("--cols", type=int, default=5, help="列数 (default: 5)")
    parser.add_argument(
        "--cell-size", type=int, default=100,
        help="セルサイズ (px、シート = rows*cell_size × cols*cell_size 想定、default: 100)",
    )
    parser.add_argument("--overwrite", action="store_true", help="既存ファイルを上書き")

    args = parser.parse_args(argv)

    if args.input:
        if not args.output:
            parser.error("--output required with --input")
        written = slice_sheet(
            args.input, args.output,
            rows=args.rows, cols=args.cols,
            cell_size=args.cell_size, overwrite=args.overwrite,
        )
        print(f"OK: {args.input} -> {args.output} ({written} cells)")
        return 0 if written > 0 else 1

    # input-sheet モード
    if not args.output_dir:
        parser.error("--output-dir required with --input-sheet")

    try:
        pairs = parse_sheet_pairs(args.input_sheet)
    except ValueError as e:
        print(f"ERROR: {e}", file=sys.stderr)
        return 1

    total = 0
    for sheet, path in pairs:
        out = args.output_dir / sheet
        written = slice_sheet(
            path, out,
            rows=args.rows, cols=args.cols,
            cell_size=args.cell_size, overwrite=args.overwrite,
        )
        total += written
        print(f"OK: {sheet}: {path} -> {out} ({written} cells)")

    print(f"Total: {total} cells written")
    return 0 if total > 0 else 1


if __name__ == "__main__":
    sys.exit(main())
