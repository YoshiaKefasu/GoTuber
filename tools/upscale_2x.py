#!/usr/bin/env python3
"""
upscale_2x.py - 2x アップスケールを 2 回適用して任意の目標サイズにリサイズするツール。

入力 PNG (例: 1254×1254) → 2x → 2x → リサイズ (例: 4500×4500)

使い方:
    # 必要環境
    # 1. realesrgan-ncnn-vulkan バイナリ (PATH に通す or スクリプトと同階層)
    #    入手: https://github.com/xinntao/Real-ESRGAN/releases
    #         (例: realesrgan-ncnn-vulkan-v0.2.0.0-windows.zip)
    # 2. モデルファイル (.param + .bin)
    #    入手: https://openmodeldb.info (例: sudo_RealESRGAN2x_Dropout)
    # 3. Python 依存: pip install Pillow

    # 実行例
    python tools/upscale_2x.py 入力フォルダ/ 出力フォルダ/ --target 4500

    # 2x を 1 回だけにしたい場合
    python tools/upscale_2x.py 入力フォルダ/ 出力フォルダ/ --scale 1 --target 2508

参考:
    - docs/01_画像生成用プロンプト.txt [6] 節
    - docs/新キャラ差し替え手順.md
"""

from __future__ import annotations

import argparse
import shutil
import subprocess
import sys
from pathlib import Path

from PIL import Image


def find_ncnn_binary() -> str:
    """realesrgan-ncnn-vulkan バイナリを探す (PATH or ./)。"""
    binary_name = "realesrgan-ncnn-vulkan.exe" if sys.platform == "win32" else "realesrgan-ncnn-vulkan"
    found = shutil.which(binary_name)
    if found:
        return found
    # PATH にない場合、スクリプトの同階層も探す
    local = Path(__file__).parent / binary_name
    if local.exists():
        return str(local)
    raise FileNotFoundError(
        f"realesrgan-ncnn-vulkan バイナリが見つかりません: {binary_name}\n"
        f"  入手: https://github.com/xinntao/Real-ESRGAN/releases\n"
        f"  配置: PATH に通すか、このスクリプトと同じフォルダに置く"
    )


def find_model_param(model_dir: Path) -> str:
    """モデルディレクトリから NCNN の .param ファイル名 (拡張子なし) を返す。"""
    if not model_dir.is_dir():
        raise NotADirectoryError(f"モデルディレクトリが存在しない: {model_dir}")
    param_files = sorted(model_dir.glob("*.param"))
    if not param_files:
        raise FileNotFoundError(f".param ファイルが見つかりません: {model_dir}")
    # 推奨: _ncnn-opt-fp16.param を優先 (軽量・最適化版)
    optimized = [p for p in param_files if "ncnn-opt" in p.stem]
    if optimized:
        return optimized[0].stem
    return param_files[0].stem


def run_2x_upscale(
    ncnn_bin: str,
    model_name: str,
    input_path: Path,
    output_path: Path,
    gpu_id: int = 0,
) -> None:
    """realesrgan-ncnn-vulkan を 1 回実行 (2x スケール)。"""
    output_path.parent.mkdir(parents=True, exist_ok=True)
    cmd = [
        ncnn_bin,
        "-i", str(input_path),
        "-o", str(output_path),
        "-n", model_name,
        "-s", "2",  # 2x
        "-f", "png",  # PNG 出力 (透過保持)
        "-g", str(gpu_id),
    ]
    print(f"  $ {' '.join(cmd)}")
    subprocess.run(cmd, check=True)


def resize_image(input_path: Path, output_path: Path, target_size: int) -> None:
    """Pillow で target_size × target_size にリサイズ (Lanczos、RGBA 保持)。"""
    output_path.parent.mkdir(parents=True, exist_ok=True)
    with Image.open(input_path) as img:
        # RGBA 保持
        if img.mode == "RGBA":
            resized = img.resize((target_size, target_size), Image.LANCZOS)
        elif img.mode == "RGB":
            resized = img.resize((target_size, target_size), Image.LANCZOS).convert("RGBA")
        else:
            # P/RGBA 以外 → 一旦 RGBA 変換
            img_rgba = img.convert("RGBA")
            resized = img_rgba.resize((target_size, target_size), Image.LANCZOS)
        resized.save(output_path, "PNG")
    print(f"  → {output_path} ({target_size}×{target_size}, {output_path.stat().st_size} bytes)")


def process_one(
    ncnn_bin: str,
    model_name: str,
    input_path: Path,
    output_dir: Path,
    scale_count: int,
    target_size: int,
    gpu_id: int,
) -> None:
    """1 枚の画像に対して N 回 2x 適用 + 目標サイズにリサイズ。"""
    print(f"\n=== {input_path.name} ===")
    current = input_path
    for i in range(scale_count):
        next_path = output_dir / f"{input_path.stem}_2x_{i + 1}.png"
        run_2x_upscale(ncnn_bin, model_name, current, next_path, gpu_id)
        current = next_path
    # 最終的に target_size にリサイズ
    final_path = output_dir / f"{input_path.stem}_{target_size}.png"
    resize_image(current, final_path, target_size)
    # 中間ファイル (2x_i.png) は最終ファイルとは別なので残してもよい
    # 不要なら削除:
    for i in range(scale_count):
        intermediate = output_dir / f"{input_path.stem}_2x_{i + 1}.png"
        if intermediate != final_path and intermediate.exists():
            intermediate.unlink()
            print(f"  clean: {intermediate.name}")


def main() -> int:
    parser = argparse.ArgumentParser(
        description="2x アップスケールを N 回適用 → 目標サイズにリサイズ",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__,
    )
    parser.add_argument("input_dir", type=Path, help="入力 PNG フォルダ")
    parser.add_argument("output_dir", type=Path, help="出力 PNG フォルダ (自動作成)")
    parser.add_argument("--model-dir", type=Path, default=None,
                        help="NCNN モデル (.param + .bin) が含まれるフォルダ (省略時: ./models)")
    parser.add_argument("--target", type=int, default=4500, help="目標サイズ (default: 4500)")
    parser.add_argument("--scale", type=int, default=2, choices=[1, 2, 3, 4],
                        help="2x 適用回数 (default: 2 = 4x 相当)")
    parser.add_argument("--gpu", type=int, default=0, help="GPU ID (default: 0, -1 で CPU)")
    args = parser.parse_args()

    if not args.input_dir.is_dir():
        print(f"ERROR: 入力ディレクトリが存在しない: {args.input_dir}", file=sys.stderr)
        return 1

    # モデルディレクトリ解決
    model_dir = args.model_dir or (Path(__file__).parent / "models")
    if not model_dir.exists():
        print(f"ERROR: モデルディレクトリが存在しない: {model_dir}", file=sys.stderr)
        print(f"  --model-dir で指定するか、{model_dir} を作成してください", file=sys.stderr)
        return 1

    try:
        ncnn_bin = find_ncnn_binary()
        model_name = find_model_param(model_dir)
    except (FileNotFoundError, NotADirectoryError) as e:
        print(f"ERROR: {e}", file=sys.stderr)
        return 1

    args.output_dir.mkdir(parents=True, exist_ok=True)
    print(f"NCNN binary: {ncnn_bin}")
    print(f"Model: {model_name}")
    print(f"Input: {args.input_dir}")
    print(f"Output: {args.output_dir}")
    print(f"Target size: {args.target}×{args.target}")
    print(f"Scale: 2x × {args.scale} = {2 ** args.scale}x")

    png_files = sorted(args.input_dir.glob("*.png"))
    if not png_files:
        print(f"ERROR: 入力ディレクトリに PNG ファイルがない: {args.input_dir}", file=sys.stderr)
        return 1

    for png in png_files:
        process_one(
            ncnn_bin=ncnn_bin,
            model_name=model_name,
            input_path=png,
            output_dir=args.output_dir,
            scale_count=args.scale,
            target_size=args.target,
            gpu_id=args.gpu,
        )

    print(f"\n完了: {len(png_files)} 枚を {args.output_dir} に出力しました。")
    return 0


if __name__ == "__main__":
    sys.exit(main())
