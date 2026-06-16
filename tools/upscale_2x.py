#!/usr/bin/env python3
"""
upscale_2x.py - 2x アップスケールを N 回適用 → 任意サイズにリサイズするツール。

バックエンド自動選択:
  1. realesrgan-ncnn-vulkan バイナリ (最速、GPU 直接)
  2. onnxruntime + .onnx モデル (Python、GPU)
  3. onnxruntime + .pth モデル (なし、CPU のみフォールバック)

使い方:
  pip install Pillow onnxruntime-gpu
  python tools/upscale_2x.py <input_dir> <output_dir> --model-dir <model_dir> --target 4500
"""

from __future__ import annotations

import argparse
import shutil
import subprocess
import sys
import time
from pathlib import Path
from typing import Any, Tuple, cast

import numpy as np
from PIL import Image


def lanczos_filter() -> int:
    """Pillow 9/10 両対応の Lanczos resampling filter。"""
    return getattr(getattr(Image, "Resampling", Image), "LANCZOS")

# ---------- バックエンド抽象 ----------

class UpscaleBackend:
    """2x アップスケール 1 回を実行する抽象インターフェース。"""
    def upscale_2x(self, input_path: Path, output_path: Path) -> None:
        raise NotImplementedError

    @property
    def name(self) -> str:
        return self.__class__.__name__


class NCNNBackend(UpscaleBackend):
    """realesrgan-ncnn-vulkan バイナリ使用。"""
    def __init__(self, ncnn_bin: str, model_dir: Path, model_name: str, gpu_id: int = 0):
        self.ncnn_bin = ncnn_bin
        self.model_dir = model_dir
        self.model_name = model_name
        self.gpu_id = gpu_id

    def upscale_2x(self, input_path: Path, output_path: Path) -> None:
        output_path.parent.mkdir(parents=True, exist_ok=True)
        cmd = [
            self.ncnn_bin,
            "-i", str(input_path),
            "-o", str(output_path),
            "-n", self.model_name,
            "-m", str(self.model_dir),
            "-s", "2",
            "-f", "png",
            "-g", str(self.gpu_id),
        ]
        subprocess.run(cmd, check=True, capture_output=True)

    @property
    def name(self) -> str:
        return f"NCNN({self.model_name})"


class ONNXBackend(UpscaleBackend):
    """onnxruntime で .onnx モデルを直接実行。

    プロバイダ自動選択 (優先順):
      1. DmlExecutionProvider (Windows + DirectX 12 GPU、NVIDIA 含む)
      2. CUDAExecutionProvider (Linux/WSL + NVIDIA CUDA toolkit)
      3. CPUExecutionProvider (フォールバック)
    """
    def __init__(self, onnx_path: Path, gpu: bool = True, device_id: int = 0):
        import onnxruntime as ort
        self.tile_size = 512
        self.tile_pad = 32
        available = ort.get_available_providers()
        providers: list = []
        if gpu:
            if "DmlExecutionProvider" in available:
                providers.append(("DmlExecutionProvider", {"device_id": device_id}))
                print(f"  ONNX: DmlExecutionProvider (DirectML, device_id={device_id}) を選択")
            elif "CUDAExecutionProvider" in available:
                providers.append(("CUDAExecutionProvider", {"device_id": device_id}))
                print(f"  ONNX: CUDAExecutionProvider (device_id={device_id}) を選択")
        providers.append("CPUExecutionProvider")
        self.session = ort.InferenceSession(str(onnx_path), providers=providers)
        self.input_name = self.session.get_inputs()[0].name
        self.output_name = self.session.get_outputs()[0].name
        # メタ情報
        inp = self.session.get_inputs()[0]
        out = self.session.get_outputs()[0]
        self.input_shape = inp.shape
        self.output_shape = out.shape
        actual_providers = self.session.get_providers()
        print(f"  ONNX: 要求プロバイダ={providers}", flush=True)
        print(f"  ONNX: 実際のプロバイダ={actual_providers}", flush=True)
        if gpu and "DmlExecutionProvider" not in actual_providers and "CUDAExecutionProvider" not in actual_providers:
            print(f"  *** WARNING: GPU 要求したが DML/CUDA いずれも実際に使われていない ***", flush=True)
        # ウォームアップ推論 (DirectML の初回 JIT コンパイルを計測から除外)
        print(f"  ONNX: ウォームアップ推論中...", flush=True)
        dummy = np.zeros((1, 3, 64, 64), dtype=np.float32)
        for _ in range(2):
            _ = self.session.run([self.output_name], {self.input_name: dummy})
        print(f"  ONNX: ウォームアップ完了", flush=True)

    def _run_rgb(self, rgb: Image.Image) -> np.ndarray:
        arr = np.asarray(rgb, dtype=np.float32) / 255.0
        # HWC -> NCHW
        arr = arr.transpose(2, 0, 1)[None]  # (1, 3, H, W)
        outputs = cast(list[Any], self.session.run([self.output_name], {self.input_name: arr}))
        result = outputs[0]
        result = np.clip(result[0].transpose(1, 2, 0), 0.0, 1.0)
        return (result * 255.0).round().astype(np.uint8)

    def _run_tiled(self, rgb: Image.Image) -> np.ndarray:
        width, height = rgb.size
        scale = 2
        output = np.zeros((height * scale, width * scale, 3), dtype=np.uint8)
        tile_count_x = (width + self.tile_size - 1) // self.tile_size
        tile_count_y = (height + self.tile_size - 1) // self.tile_size
        total = tile_count_x * tile_count_y
        done = 0
        for y0 in range(0, height, self.tile_size):
            y1 = min(y0 + self.tile_size, height)
            py0 = max(y0 - self.tile_pad, 0)
            py1 = min(y1 + self.tile_pad, height)
            for x0 in range(0, width, self.tile_size):
                x1 = min(x0 + self.tile_size, width)
                px0 = max(x0 - self.tile_pad, 0)
                px1 = min(x1 + self.tile_pad, width)
                done += 1
                print(
                    f"    tile {done}/{total}: "
                    f"core=({x0},{y0})-({x1},{y1}) "
                    f"pad=({px0},{py0})-({px1},{py1})",
                    flush=True,
                )
                patch = rgb.crop((px0, py0, px1, py1))
                patch_out = self._run_rgb(patch)
                crop_left = (x0 - px0) * scale
                crop_top = (y0 - py0) * scale
                crop_right = crop_left + (x1 - x0) * scale
                crop_bottom = crop_top + (y1 - y0) * scale
                output[y0 * scale:y1 * scale, x0 * scale:x1 * scale, :] = patch_out[
                    crop_top:crop_bottom,
                    crop_left:crop_right,
                    :,
                ]
        return output

    def upscale_2x(self, input_path: Path, output_path: Path) -> None:
        output_path.parent.mkdir(parents=True, exist_ok=True)
        with Image.open(input_path) as img:
            rgba = img.convert("RGBA")
            alpha = rgba.getchannel("A")
            rgb = rgba.convert("RGB")
            width, height = rgb.size
            if max(width, height) > self.tile_size:
                print(
                    f"    ONNX tiled inference: {width}×{height}, "
                    f"tile={self.tile_size}, pad={self.tile_pad}",
                    flush=True,
                )
                rgb_result = self._run_tiled(rgb)
            else:
                rgb_result = self._run_rgb(rgb)
            out_h, out_w = rgb_result.shape[:2]
            alpha_result = alpha.resize((out_w, out_h), lanczos_filter())
            result_img = Image.fromarray(rgb_result, "RGB").convert("RGBA")
            result_img.putalpha(alpha_result)
            result_img.save(output_path, "PNG")

    @property
    def name(self) -> str:
        return f"ONNX({self.input_name} -> {self.output_name}, tile={self.tile_size})"


def find_ncnn_binary() -> str | None:
    binary_name = "realesrgan-ncnn-vulkan.exe" if sys.platform == "win32" else "realesrgan-ncnn-vulkan"
    found = shutil.which(binary_name)
    if found:
        return found
    local = Path(__file__).parent / binary_name
    if local.exists():
        return str(local)
    return None


def find_onnx_model(model_dir: Path) -> Path | None:
    """モデルディレクトリから最適な .onnx ファイルを選ぶ。"""
    if not model_dir.is_dir():
        return None
    onnx_files = sorted(model_dir.glob("*.onnx"))
    if not onnx_files:
        return None
    # 優先順位: _opset14_ncnn-opt (最適化版) > _opset14 > _opset13/15/16 > その他
    for keyword in ["_ncnn-opt", "_opset14", "_opset13", "_opset15", "_opset16"]:
        matches = [f for f in onnx_files if keyword in f.stem]
        if matches:
            return matches[0]
    return onnx_files[0]


def select_backend(model_dir: Path, use_gpu: bool = True, device_id: int = 0) -> UpscaleBackend:
    """利用可能なバックエンドを自動選択。"""
    # 1. NCNN バイナリ
    ncnn_bin = find_ncnn_binary()
    if ncnn_bin:
        param_files = sorted(model_dir.glob("*.param")) if model_dir.is_dir() else []
        if param_files:
            # ncnn-opt 優先
            optimized = [p for p in param_files if "ncnn-opt" in p.stem]
            model_name = optimized[0].stem if optimized else param_files[0].stem
            return NCNNBackend(ncnn_bin, model_dir, model_name, gpu_id=device_id)

    # 2. ONNX モデル
    onnx_path = find_onnx_model(model_dir)
    if onnx_path:
        try:
            return ONNXBackend(onnx_path, gpu=use_gpu, device_id=device_id)
        except Exception as e:
            print(f"  ONNX バックエンド初期化失敗: {e}")

    # 3. 失敗
    raise RuntimeError(
        f"利用可能なバックエンドがない:\n"
        f"  - NCNN バイナリ: {find_ncnn_binary() or '未検出'}\n"
        f"  - ONNX モデル: {find_onnx_model(model_dir) or '未検出'} ({model_dir})\n"
        f"NCNN バイナリを入手: https://github.com/xinntao/Real-ESRGAN/releases\n"
        f"pip install onnxruntime-gpu Pillow で Python 実行も可能"
    )


# ---------- メイン処理 ----------

def resize_image(input_path: Path, output_path: Path, target_size: int) -> None:
    """Lanczos リサイズ (RGBA 保持)。"""
    output_path.parent.mkdir(parents=True, exist_ok=True)
    with Image.open(input_path) as img:
        if img.mode != "RGBA":
            img = img.convert("RGBA")
        resized = img.resize((target_size, target_size), lanczos_filter())
        resized.save(output_path, "PNG")
    print(f"  → {output_path.name} ({target_size}×{target_size}, {output_path.stat().st_size} bytes)")


def process_one(
    backend: UpscaleBackend,
    input_path: Path,
    output_dir: Path,
    scale_count: int,
    target_size: int,
) -> None:
    print(f"\n=== {input_path.name} ({backend.name}) ===")
    t0 = time.time()
    current = input_path
    for i in range(scale_count):
        next_path = output_dir / f"{input_path.stem}_2x_{i + 1}.png"
        print(f"  [{i + 1}/{scale_count}] 2x: {current.name} → {next_path.name}")
        ts = time.time()
        backend.upscale_2x(current, next_path)
        te = time.time()
        print(f"    ({te - ts:.1f}s)")
        current = next_path
    final_path = output_dir / f"{input_path.stem}_{target_size}.png"
    print(f"  resize → {final_path.name}")
    resize_image(current, final_path, target_size)
    # 中間ファイル削除
    for i in range(scale_count):
        intermediate = output_dir / f"{input_path.stem}_2x_{i + 1}.png"
        if intermediate != final_path and intermediate.exists():
            intermediate.unlink()
    print(f"  合計: {time.time() - t0:.1f}s")


def main() -> int:
    parser = argparse.ArgumentParser(
        description="2x アップスケールを N 回 → 目標サイズにリサイズ",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__,
    )
    parser.add_argument("input_dir", type=Path, help="入力 PNG フォルダ")
    parser.add_argument("output_dir", type=Path, help="出力 PNG フォルダ (自動作成)")
    parser.add_argument("--model-dir", type=Path, default=None,
                        help="NCNN モデル (.param + .bin) または ONNX モデル (.onnx) が含まれるフォルダ")
    parser.add_argument("--target", type=int, default=4500, help="目標サイズ (default: 4500)")
    parser.add_argument("--scale", type=int, default=2, choices=[1, 2, 3, 4],
                        help="2x 適用回数 (default: 2 = 4x 相当)")
    parser.add_argument("--gpu", action="store_true", default=True, help="GPU を使う (default)")
    parser.add_argument("--no-gpu", dest="gpu", action="store_false", help="CPU のみ")
    parser.add_argument("--device-id", type=int, default=0,
                        help="GPU device ID (DirectML: 0=primary GPU, 1=secondary GPU, RTX 2060 は多くの場合 1)")
    parser.add_argument("--limit", type=int, default=0, help="処理する最大枚数 (0=無制限)")
    args = parser.parse_args()

    if not args.input_dir.is_dir():
        print(f"ERROR: 入力ディレクトリが存在しない: {args.input_dir}", file=sys.stderr)
        return 1

    model_dir = args.model_dir or (Path(__file__).parent / "models")
    if not model_dir.exists():
        print(f"ERROR: モデルディレクトリが存在しない: {model_dir}", file=sys.stderr)
        return 1

    try:
        backend = select_backend(model_dir, use_gpu=args.gpu, device_id=args.device_id)
    except RuntimeError as e:
        print(f"ERROR: {e}", file=sys.stderr)
        return 1

    args.output_dir.mkdir(parents=True, exist_ok=True)
    print(f"Backend: {backend.name}")
    print(f"Input: {args.input_dir}")
    print(f"Output: {args.output_dir}")
    print(f"Target: {args.target}×{args.target}, 2x × {args.scale} = {2 ** args.scale}x")

    png_files = sorted(args.input_dir.glob("*.png"))
    if not png_files:
        print(f"ERROR: 入力ディレクトリに PNG ファイルがない: {args.input_dir}", file=sys.stderr)
        return 1
    if args.limit > 0:
        png_files = png_files[:args.limit]
    print(f"処理対象: {len(png_files)} 枚")

    t_all = time.time()
    for png in png_files:
        process_one(backend, png, args.output_dir, args.scale, args.target)
    print(f"\n=== 全完了 ({time.time() - t_all:.1f}s) ===")
    return 0


if __name__ == "__main__":
    sys.exit(main())
