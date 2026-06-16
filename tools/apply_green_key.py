#!/usr/bin/env python3
"""Chroma key (green screen) 適用スクリプト。

#00FF00 背景の PNG を RGBA に変換し、緑のピクセルを α=0 にする。
HSV 色空間で判定することで、AI が「#00FF00 周辺」しか塗ってない場合にも頑健。

使い方:
  python tools/apply_green_key.py <input_dir> <output_dir>

  <input_dir>/*#00FF00.png  →  <output_dir>/<original>.png (RGBA 透過)
"""

import argparse
import sys
from pathlib import Path
from typing import Any, cast

from PIL import Image

PixelAccess = Any  # Pillow 型スタブ問題回避


def apply_chroma_key(
    src: Path,
    dst: Path,
    threshold: int = 80,
) -> tuple[int, int, float]:
    """Chroma key (緑) を α=0 にする。

    厳密 #00FF00 だけでなく、G が支配的な色 (R, B ともに G より threshold 以上低い)
    をすべて透過。AI 生成の「緑背景の周辺色」も対応。

    Args:
        src: 入力 PNG (RGB or RGBA)
        dst: 出力 PNG (RGBA)
        threshold: G - max(R, B) の最小値。0 だと完全一致、80 推奨。

    Returns:
        (全ピクセル数, 透明化したピクセル数, 透明化率 0.0-1.0)
    """
    img = Image.open(src)
    if img.mode != "RGBA":
        img = img.convert("RGBA")

    pixels = cast(PixelAccess, img.load())  # type: ignore[arg-type]
    if pixels is None:
        raise RuntimeError("load failed")

    w, h = img.size
    total = w * h
    transparent = 0

    for y in range(h):
        for x in range(w):
            rgba = pixels[x, y]
            r, g, b = int(rgba[0]), int(rgba[1]), int(rgba[2])
            a = int(rgba[3]) if len(rgba) > 3 else 255
            if a == 0:
                continue
            # 緑判定
            if g - max(r, b) >= threshold and g >= 60:
                pixels[x, y] = (r, g, b, 0)
                transparent += 1

    dst.parent.mkdir(parents=True, exist_ok=True)
    img.save(dst, "PNG")
    return total, transparent, transparent / total if total else 0


def main() -> int:
    parser = argparse.ArgumentParser(description="Green screen (#00FF00) chroma key 適用")
    parser.add_argument("input", help="入力ディレクトリ")
    parser.add_argument("output", help="出力ディレクトリ")
    parser.add_argument("--threshold", type=int, default=80,
                        help="G - max(R,B) の最小値 (推奨: 80, 厳密: 0)")
    args = parser.parse_args()

    src_dir = Path(args.input)
    dst_dir = Path(args.output)

    if not src_dir.is_dir():
        print(f"ERROR: {src_dir} が存在しない", file=sys.stderr)
        return 1

    pngs = sorted(src_dir.glob("*#00FF00.png"))
    if not pngs:
        print(f"ERROR: {src_dir} に *#00FF00.png がない", file=sys.stderr)
        return 1

    print(f"入力: {src_dir} ({len(pngs)} 枚)")
    print(f"出力: {dst_dir}")
    print(f"chroma threshold: {args.threshold}")
    print()

    grand_total = 0
    grand_transparent = 0
    for png in pngs:
        out = dst_dir / png.name
        total, transparent, ratio = apply_chroma_key(png, out, threshold=args.threshold)
        grand_total += total
        grand_transparent += transparent
        print(f"OK   {png.name}  →  透明化 {transparent:,}/{total:,} ({ratio*100:.1f}%)")

    if grand_total:
        ratio = grand_transparent / grand_total
        print(f"\n合計: 透明化 {grand_transparent:,}/{grand_total:,} ({ratio*100:.1f}%)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
