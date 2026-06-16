#!/usr/bin/env python3
"""完全不透明な背景を α=0 (透明) に変換するツール。

AI 画像生成で「背景はシンプルなグレー」とプロンプトしたが、生成画像は
完全不透明 (α=255) のままになる。slice_character_sheets.py の
remove-gray-residue は「α が < 255 のグレー」を落とすので、その前段として
完全不透明な背景を透明化する。

動作:
  1. 4 隅 (インセット 50px) の色の中央値を「背景色」とみなす
  2. 各ピクセルで、背景色との L∞ 距離 (max(|R-bgR|, |G-bgG|, |B-bgB|))
     が delta 以下なら α=0 に変更
  3. キャラ部分 (背景色から離れている) は触らない

使い方:
  python tools/remove_gray_background.py <input_dir> <output_dir> --delta 30
"""

import argparse
import sys
from pathlib import Path
from typing import Any, cast

from PIL import Image

PixelAccess = Any  # Pillow の型スタブが PixelAccess を export してないため Any で代用


def sample_background_color(
    img: Image.Image,
    corner_inset: int = 50,
) -> tuple[int, int, int]:
    """4 隅の色をサンプリングして中央値を返す (背景色の代表値)。"""
    w, h = img.size
    samples: list[tuple[int, int, int]] = []
    for cx, cy in [
        (corner_inset, corner_inset),
        (w - corner_inset, corner_inset),
        (corner_inset, h - corner_inset),
        (w - corner_inset, h - corner_inset),
    ]:
        raw = cast(Any, img.convert("RGB").getpixel((cx, cy)))
        samples.append((int(raw[0]), int(raw[1]), int(raw[2])))
    rs = sorted(s[0] for s in samples)
    gs = sorted(s[1] for s in samples)
    bs = sorted(s[2] for s in samples)
    mid = len(samples) // 2
    return (rs[mid], gs[mid], bs[mid])


def remove_gray_background(
    src: Path,
    dst: Path,
    delta: int = 30,
) -> tuple[int, int]:
    """完全不透明な背景を透明化する。

    動作:
      1. 4 隅から背景色をサンプル (中央値)
      2. 各ピクセルで、背景色との L∞ 距離 (max(|R-bgR|, |G-bgG|, |B-bgB|))
         が delta 以下なら α=0
      3. キャラ部分 (背景色から離れている) は触らない

    Args:
        src: 入力 PNG (RGBA or RGB)
        dst: 出力 PNG (RGBA)
        delta: 背景色とみなす許容差 (推奨: 30)。
               0 だと厳密一致、50 だとグラデーション背景も透明化。

    Returns:
        (全ピクセル数, 透明化したピクセル数)
    """
    img = Image.open(src)
    if img.mode != "RGBA":
        img = img.convert("RGBA")

    # 背景色を 4 隅からサンプリング
    bg = sample_background_color(img)
    bg_r, bg_g, bg_b = bg
    print(f"  背景色サンプル (4 隅中央値): RGB={bg}")

    pixels = cast(PixelAccess, img.load())  # type: ignore[arg-type]
    if pixels is None:
        raise RuntimeError("load failed")

    w, h = img.size
    total = w * h
    transparent = 0

    for y in range(h):
        for x in range(w):
            rgba = pixels[x, y]
            r, g, b, a = int(rgba[0]), int(rgba[1]), int(rgba[2]), int(rgba[3])
            # 背景色との距離を計算
            dist = max(abs(r - bg_r), abs(g - bg_g), abs(b - bg_b))
            if dist <= delta:
                # 背景色と一致 → 完全透明
                if a != 0:
                    pixels[x, y] = (r, g, b, 0)
                    transparent += 1
            # 背景色に近い (delta 以内) ピクセルは問答無用で α=0
            # 背景色から遠い (delta 超過) → 触らない (キャラ)

    dst.parent.mkdir(parents=True, exist_ok=True)
    img.save(dst, "PNG")
    return total, transparent


def main() -> int:
    parser = argparse.ArgumentParser(description="グレー背景を透明化")
    parser.add_argument("input", help="入力ディレクトリ (PNG を含む)")
    parser.add_argument("output", help="出力ディレクトリ")
    parser.add_argument("--delta", type=int, default=4)
    args = parser.parse_args()

    src_dir = Path(args.input)
    dst_dir = Path(args.output)

    if not src_dir.is_dir():
        print(f"ERROR: {src_dir} が存在しない", file=sys.stderr)
        return 1

    pngs = sorted(src_dir.glob("*.png"))
    if not pngs:
        print(f"ERROR: {src_dir} に PNG がない", file=sys.stderr)
        return 1

    print(f"入力: {src_dir} ({len(pngs)} 枚)")
    print(f"出力: {dst_dir}")
    print(f"グレー判定 delta: {args.delta}")
    print()

    grand_total = 0
    grand_transparent = 0
    for png in pngs:
        out = dst_dir / png.name
        total, transparent = remove_gray_background(png, out, delta=args.delta)
        grand_total += total
        grand_transparent += transparent
        pct = 100.0 * transparent / total if total else 0
        print(f"OK   {png.name}  → 透明化 {transparent:,}/{total:,} ({pct:.2f}%)")

    if grand_total:
        pct = 100.0 * grand_transparent / grand_total
        print(f"\n合計: 透明化 {grand_transparent:,}/{grand_total:,} ({pct:.2f}%)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
