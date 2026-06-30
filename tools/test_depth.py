#!/usr/bin/env python3
"""
test_depth.py - Phase 3.6 Depth Map Generator テスト

gotuber_creator.py の validate-depth / generate-depth コマンドのテスト。
Pillow が利用可能な環境で実行すること。

使い方:
  python -m pytest tools/test_depth.py -v
  python tools/test_depth.py
"""

from __future__ import annotations

import argparse
import shutil
import sys
import tempfile
from pathlib import Path

# gotuber_creator.py を import できるように tools/ を path に追加
TOOLS_DIR = Path(__file__).resolve().parent
if str(TOOLS_DIR) not in sys.path:
    sys.path.insert(0, str(TOOLS_DIR))

import gotuber_creator as gc

try:
    from PIL import Image

    HAS_PILLOW = True
except ImportError:
    HAS_PILLOW = False


# ---------- テスト用ヘルパー ----------


def _make_character_tree(base: Path, sheets: list[str] | None = None) -> None:
    """テスト用のキャラクターディレクトリ構造を作る。

    各 sheet に 25 枚の 1200x1200 RGBA WebP を配置する。
    """
    if sheets is None:
        sheets = list(gc.SHEET_NAMES)

    for sheet in sheets:
        sheet_dir = base / sheet
        sheet_dir.mkdir(parents=True, exist_ok=True)
        for r in range(gc.ROWS):
            for c in range(gc.COLS):
                img = Image.new("RGBA", (1200, 1200), (100, 150, 200, 255))
                img.save(sheet_dir / gc.cell_filename(r, c, "webp"), "WEBP")


def _make_depth_tree(
    base: Path, sheets: list[str] | None = None, size: tuple[int, int] = (1200, 1200)
) -> None:
    """テスト用の depth map ディレクトリ構造を作る。"""
    if sheets is None:
        sheets = list(gc.SHEET_NAMES)

    for sheet in sheets:
        depth_dir = base / sheet / gc.DEPTH_DIR_NAME
        depth_dir.mkdir(parents=True, exist_ok=True)
        for r in range(gc.ROWS):
            for c in range(gc.COLS):
                img = Image.new("L", size, 128)
                img.save(depth_dir / gc.cell_filename(r, c, "png"), "PNG")


# ---------- validate-depth テスト ----------


def test_validate_depth_all_sheets_pass() -> None:
    """全 6 sheet × 25 枚 = 150 枚の depth map が検証通過する。"""
    if not HAS_PILLOW:
        print("SKIP: Pillow not installed")
        return

    with tempfile.TemporaryDirectory() as tmpdir:
        base = Path(tmpdir) / "testchar"
        base.mkdir()
        _make_depth_tree(base)

        args = argparse.Namespace(input=str(base), sheets=None)
        rc = gc.cmd_validate_depth(args)
        assert rc == 0, f"Expected exit 0, got {rc}"


def test_validate_depth_single_sheet() -> None:
    """--sheets A で sheet A のみ検証する。"""
    if not HAS_PILLOW:
        print("SKIP: Pillow not installed")
        return

    with tempfile.TemporaryDirectory() as tmpdir:
        base = Path(tmpdir) / "testchar"
        base.mkdir()
        _make_depth_tree(base, sheets=["A"])

        args = argparse.Namespace(input=str(base), sheets="A")
        rc = gc.cmd_validate_depth(args)
        assert rc == 0


def test_validate_depth_missing_dir() -> None:
    """depth/ ディレクトリがない場合はエラー。"""
    if not HAS_PILLOW:
        print("SKIP: Pillow not installed")
        return

    with tempfile.TemporaryDirectory() as tmpdir:
        base = Path(tmpdir) / "testchar"
        base.mkdir()
        # A ディレクトリは作るが depth/ は作らない
        (base / "A").mkdir()

        args = argparse.Namespace(input=str(base), sheets="A")
        rc = gc.cmd_validate_depth(args)
        assert rc == 1


def test_validate_depth_missing_files() -> None:
    """25 枚中一部欠落場合はエラー。"""
    if not HAS_PILLOW:
        print("SKIP: Pillow not installed")
        return

    with tempfile.TemporaryDirectory() as tmpdir:
        base = Path(tmpdir) / "testchar"
        base.mkdir()
        depth_dir = base / "A" / gc.DEPTH_DIR_NAME
        depth_dir.mkdir(parents=True)

        # r0c0 と r0c1 のみ配置
        for r, c in [(0, 0), (0, 1)]:
            img = Image.new("L", (1200, 1200), 128)
            img.save(depth_dir / gc.cell_filename(r, c, "png"), "PNG")

        args = argparse.Namespace(input=str(base), sheets="A")
        rc = gc.cmd_validate_depth(args)
        assert rc == 1


def test_validate_depth_wrong_size() -> None:
    """サイズが 1200x1200 でない場合はエラー。"""
    if not HAS_PILLOW:
        print("SKIP: Pillow not installed")
        return

    with tempfile.TemporaryDirectory() as tmpdir:
        base = Path(tmpdir) / "testchar"
        base.mkdir()
        _make_depth_tree(base, sheets=["A"], size=(900, 900))

        args = argparse.Namespace(input=str(base), sheets="A")
        rc = gc.cmd_validate_depth(args)
        assert rc == 1


def test_validate_depth_grayscale_alpha_accepted() -> None:
    """grayscale+alpha (LA) モードでも検証通過する。"""
    if not HAS_PILLOW:
        print("SKIP: Pillow not installed")
        return

    with tempfile.TemporaryDirectory() as tmpdir:
        base = Path(tmpdir) / "testchar"
        base.mkdir()

        for sheet in ["A"]:
            depth_dir = base / sheet / gc.DEPTH_DIR_NAME
            depth_dir.mkdir(parents=True)
            for r in range(gc.ROWS):
                for c in range(gc.COLS):
                    img = Image.new("LA", (1200, 1200), (128, 255))
                    img.save(depth_dir / gc.cell_filename(r, c, "png"), "PNG")

        args = argparse.Namespace(input=str(base), sheets="A")
        rc = gc.cmd_validate_depth(args)
        assert rc == 0


def test_validate_depth_rgb_grayscale_equivalent_passes() -> None:
    """RGB でも R=G=B (grayscale-equivalent) なら検証通過。"""
    if not HAS_PILLOW:
        print("SKIP: Pillow not installed")
        return

    with tempfile.TemporaryDirectory() as tmpdir:
        base = Path(tmpdir) / "testchar"
        base.mkdir()
        depth_dir = base / "A" / gc.DEPTH_DIR_NAME
        depth_dir.mkdir(parents=True)
        for r in range(gc.ROWS):
            for c in range(gc.COLS):
                # R=G=B=128 → grayscale-equivalent
                img = Image.new("RGB", (1200, 1200), (128, 128, 128))
                img.save(depth_dir / gc.cell_filename(r, c, "png"), "PNG")

        args = argparse.Namespace(input=str(base), sheets="A")
        rc = gc.cmd_validate_depth(args)
        assert rc == 0, f"RGB grayscale-equivalent should pass, got {rc}"


def test_validate_depth_rgba_grayscale_equivalent_passes() -> None:
    """RGBA でも R=G=B (grayscale-equivalent) なら検証通過。"""
    if not HAS_PILLOW:
        print("SKIP: Pillow not installed")
        return

    with tempfile.TemporaryDirectory() as tmpdir:
        base = Path(tmpdir) / "testchar"
        base.mkdir()
        depth_dir = base / "A" / gc.DEPTH_DIR_NAME
        depth_dir.mkdir(parents=True)
        for r in range(gc.ROWS):
            for c in range(gc.COLS):
                # R=G=B=128, A=200 → grayscale-equivalent with alpha
                img = Image.new("RGBA", (1200, 1200), (128, 128, 128, 200))
                img.save(depth_dir / gc.cell_filename(r, c, "png"), "PNG")

        args = argparse.Namespace(input=str(base), sheets="A")
        rc = gc.cmd_validate_depth(args)
        assert rc == 0, f"RGBA grayscale-equivalent should pass, got {rc}"


def test_validate_depth_rgb_arbitrary_color_rejected() -> None:
    """RGB で R≠G の arbitrary color は拒否。"""
    if not HAS_PILLOW:
        print("SKIP: Pillow not installed")
        return

    with tempfile.TemporaryDirectory() as tmpdir:
        base = Path(tmpdir) / "testchar"
        base.mkdir()
        depth_dir = base / "A" / gc.DEPTH_DIR_NAME
        depth_dir.mkdir(parents=True)
        for r in range(gc.ROWS):
            for c in range(gc.COLS):
                # R≠G → arbitrary color
                img = Image.new("RGB", (1200, 1200), (200, 100, 50))
                img.save(depth_dir / gc.cell_filename(r, c, "png"), "PNG")

        args = argparse.Namespace(input=str(base), sheets="A")
        rc = gc.cmd_validate_depth(args)
        assert rc == 1, f"RGB arbitrary color should fail, got {rc}"


def test_validate_depth_rgba_arbitrary_color_rejected() -> None:
    """RGBA で R≠G の arbitrary color は拒否。"""
    if not HAS_PILLOW:
        print("SKIP: Pillow not installed")
        return

    with tempfile.TemporaryDirectory() as tmpdir:
        base = Path(tmpdir) / "testchar"
        base.mkdir()
        depth_dir = base / "A" / gc.DEPTH_DIR_NAME
        depth_dir.mkdir(parents=True)
        for r in range(gc.ROWS):
            for c in range(gc.COLS):
                # R≠G → arbitrary color with alpha
                img = Image.new("RGBA", (1200, 1200), (200, 100, 50, 255))
                img.save(depth_dir / gc.cell_filename(r, c, "png"), "PNG")

        args = argparse.Namespace(input=str(base), sheets="A")
        rc = gc.cmd_validate_depth(args)
        assert rc == 1, f"RGBA arbitrary color should fail, got {rc}"


def test_validate_depth_palette_rejected() -> None:
    """パレットモード (P) は拒否。"""
    if not HAS_PILLOW:
        print("SKIP: Pillow not installed")
        return

    with tempfile.TemporaryDirectory() as tmpdir:
        base = Path(tmpdir) / "testchar"
        base.mkdir()
        depth_dir = base / "A" / gc.DEPTH_DIR_NAME
        depth_dir.mkdir(parents=True)
        for r in range(gc.ROWS):
            for c in range(gc.COLS):
                img = Image.new("P", (1200, 1200), 0)
                img.save(depth_dir / gc.cell_filename(r, c, "png"), "PNG")

        args = argparse.Namespace(input=str(base), sheets="A")
        rc = gc.cmd_validate_depth(args)
        assert rc == 1, f"Palette mode should fail, got {rc}"


def test_validate_depth_extra_files() -> None:
    """想定外のファイルがある場合はエラー。"""
    if not HAS_PILLOW:
        print("SKIP: Pillow not installed")
        return

    with tempfile.TemporaryDirectory() as tmpdir:
        base = Path(tmpdir) / "testchar"
        base.mkdir()
        _make_depth_tree(base, sheets=["A"])

        # 想定外のファイルを追加
        depth_dir = base / "A" / gc.DEPTH_DIR_NAME
        img = Image.new("L", (1200, 1200), 128)
        img.save(depth_dir / "extra.png", "PNG")

        args = argparse.Namespace(input=str(base), sheets="A")
        rc = gc.cmd_validate_depth(args)
        assert rc == 1


# ---------- generate-depth テスト ----------


def test_generate_depth_heuristic_single_sheet() -> None:
    """heuristic バックエンドで sheet A の 25 枚を生成する。"""
    if not HAS_PILLOW:
        print("SKIP: Pillow not installed")
        return

    with tempfile.TemporaryDirectory() as tmpdir:
        base = Path(tmpdir) / "testchar"
        base.mkdir()
        _make_character_tree(base, sheets=["A"])

        args = argparse.Namespace(
            input=str(base), sheets="A", backend="heuristic",
            ext="webp", force=False,
        )
        rc = gc.cmd_generate_depth(args)
        assert rc == 0

        # 出力されたか確認
        depth_dir = base / "A" / gc.DEPTH_DIR_NAME
        assert depth_dir.is_dir()
        files = list(depth_dir.glob("r*.png"))
        assert len(files) == 25


def test_generate_depth_heuristic_all_sheets() -> None:
    """heuristic で全 6 sheet を生成する。"""
    if not HAS_PILLOW:
        print("SKIP: Pillow not installed")
        return

    with tempfile.TemporaryDirectory() as tmpdir:
        base = Path(tmpdir) / "testchar"
        base.mkdir()
        _make_character_tree(base)

        args = argparse.Namespace(
            input=str(base), sheets=None, backend="heuristic",
            ext="webp", force=False,
        )
        rc = gc.cmd_generate_depth(args)
        assert rc == 0

        # 全 150 枚確認
        total = 0
        for sheet in gc.SHEET_NAMES:
            depth_dir = base / sheet / gc.DEPTH_DIR_NAME
            total += len(list(depth_dir.glob("r*.png")))
        assert total == 150


def test_generate_depth_skip_existing() -> None:
    """既存 depth map は --force なしではスキップされる。"""
    if not HAS_PILLOW:
        print("SKIP: Pillow not installed")
        return

    with tempfile.TemporaryDirectory() as tmpdir:
        base = Path(tmpdir) / "testchar"
        base.mkdir()
        _make_character_tree(base, sheets=["A"])
        _make_depth_tree(base, sheets=["A"])

        # 既存ファイルの最終変更時刻を記録
        depth_dir = base / "A" / gc.DEPTH_DIR_NAME
        existing = depth_dir / "r0c0.png"
        mtime_before = existing.stat().st_mtime

        args = argparse.Namespace(
            input=str(base), sheets="A", backend="heuristic",
            ext="webp", force=False,
        )
        rc = gc.cmd_generate_depth(args)
        assert rc == 0

        # 変更されていないことを確認
        mtime_after = existing.stat().st_mtime
        assert mtime_before == mtime_after


def test_generate_depth_force_overwrite() -> None:
    """--force で既存 depth map を上書きする。"""
    if not HAS_PILLOW:
        print("SKIP: Pillow not installed")
        return

    with tempfile.TemporaryDirectory() as tmpdir:
        base = Path(tmpdir) / "testchar"
        base.mkdir()
        _make_character_tree(base, sheets=["A"])
        _make_depth_tree(base, sheets=["A"])

        depth_dir = base / "A" / gc.DEPTH_DIR_NAME
        existing = depth_dir / "r0c0.png"
        mtime_before = existing.stat().st_mtime

        import time
        time.sleep(0.05)  # mtime 確保

        args = argparse.Namespace(
            input=str(base), sheets="A", backend="heuristic",
            ext="webp", force=True,
        )
        rc = gc.cmd_generate_depth(args)
        assert rc == 0

        mtime_after = existing.stat().st_mtime
        assert mtime_after > mtime_before


def test_generate_depth_output_is_grayscale_png() -> None:
    """生成された depth map は grayscale PNG として読み込める。"""
    if not HAS_PILLOW:
        print("SKIP: Pillow not installed")
        return

    with tempfile.TemporaryDirectory() as tmpdir:
        base = Path(tmpdir) / "testchar"
        base.mkdir()
        _make_character_tree(base, sheets=["A"])

        args = argparse.Namespace(
            input=str(base), sheets="A", backend="heuristic",
            ext="webp", force=False,
        )
        rc = gc.cmd_generate_depth(args)
        assert rc == 0

        # 全ファイルを検証
        depth_dir = base / "A" / gc.DEPTH_DIR_NAME
        for f in depth_dir.glob("r*.png"):
            img = Image.open(f)
            assert img.size == (1200, 1200), f"{f.name}: size {img.size}"
            assert img.mode in {"L", "LA"}, f"{f.name}: mode {img.mode}"
            img.close()


def test_generate_depth_manual_existing_ok() -> None:
    """manual モード: 既存ファイルがある場合はスキップ。"""
    if not HAS_PILLOW:
        print("SKIP: Pillow not installed")
        return

    with tempfile.TemporaryDirectory() as tmpdir:
        base = Path(tmpdir) / "testchar"
        base.mkdir()
        _make_character_tree(base, sheets=["A"])
        _make_depth_tree(base, sheets=["A"])

        args = argparse.Namespace(
            input=str(base), sheets="A", backend="manual",
            ext="webp", force=False,
        )
        rc = gc.cmd_generate_depth(args)
        assert rc == 0


def test_generate_depth_manual_missing_error() -> None:
    """manual モード: depth map がない場合はエラー。"""
    if not HAS_PILLOW:
        print("SKIP: Pillow not installed")
        return

    with tempfile.TemporaryDirectory() as tmpdir:
        base = Path(tmpdir) / "testchar"
        base.mkdir()
        _make_character_tree(base, sheets=["A"])

        args = argparse.Namespace(
            input=str(base), sheets="A", backend="manual",
            ext="webp", force=False,
        )
        rc = gc.cmd_generate_depth(args)
        assert rc == 1


def test_generate_depth_nonexistent_input() -> None:
    """存在しない入力ディレクトリでエラー。"""
    if not HAS_PILLOW:
        print("SKIP: Pillow not installed")
        return

    args = argparse.Namespace(
        input="/nonexistent/path", sheets="A", backend="heuristic",
        ext="webp", force=False,
    )
    rc = gc.cmd_generate_depth(args)
    assert rc == 1


def test_validate_depth_nonexistent_input() -> None:
    """存在しない入力ディレクトリでエラー。"""
    if not HAS_PILLOW:
        print("SKIP: Pillow not installed")
        return

    args = argparse.Namespace(input="/nonexistent/path", sheets=None)
    rc = gc.cmd_validate_depth(args)
    assert rc == 1


# ---------- CLI 統合テスト ----------


def test_cli_validate_depth_help() -> None:
    """validate-depth のヘルプが表示できる。"""
    args = argparse.Namespace()
    # parse 完了後の Namespace を渡す代わりに、関数の docstring を確認
    assert "Phase 4" in gc.cmd_validate_depth.__doc__


def test_cli_generate_depth_help() -> None:
    """generate-depth のヘルプが表示できる。"""
    assert "Morph Renderer" in gc.cmd_generate_depth.__doc__


# ---------- メイン ----------


def run_all_tests() -> int:
    """全テストを実行し、結果を報告する。"""
    tests = [
        test_validate_depth_all_sheets_pass,
        test_validate_depth_single_sheet,
        test_validate_depth_missing_dir,
        test_validate_depth_missing_files,
        test_validate_depth_wrong_size,
        test_validate_depth_grayscale_alpha_accepted,
        test_validate_depth_rgb_grayscale_equivalent_passes,
        test_validate_depth_rgba_grayscale_equivalent_passes,
        test_validate_depth_rgb_arbitrary_color_rejected,
        test_validate_depth_rgba_arbitrary_color_rejected,
        test_validate_depth_palette_rejected,
        test_validate_depth_extra_files,
        test_generate_depth_heuristic_single_sheet,
        test_generate_depth_heuristic_all_sheets,
        test_generate_depth_skip_existing,
        test_generate_depth_force_overwrite,
        test_generate_depth_output_is_grayscale_png,
        test_generate_depth_manual_existing_ok,
        test_generate_depth_manual_missing_error,
        test_generate_depth_nonexistent_input,
        test_validate_depth_nonexistent_input,
        test_cli_validate_depth_help,
        test_cli_generate_depth_help,
    ]

    passed = 0
    failed = 0
    skipped = 0

    for test_fn in tests:
        name = test_fn.__name__
        try:
            result = test_fn()
            if result == "skip":
                skipped += 1
                print(f"  SKIP  {name}")
            else:
                passed += 1
                print(f"  PASS  {name}")
        except AssertionError as e:
            failed += 1
            print(f"  FAIL  {name}: {e}")
        except Exception as e:
            failed += 1
            print(f"  ERROR {name}: {e}")

    print(f"\n{'='*60}")
    print(f"Results: {passed} passed, {failed} failed, {skipped} skipped")
    print(f"{'='*60}")
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(run_all_tests())
