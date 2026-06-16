"""
slice_character_sheets_test.py - スプライトシート分割ツールのテスト

実行:
    pip install -r tools/requirements.txt
    python -m pytest tools/slice_character_sheets_test.py -v
    # または
    python tools/slice_character_sheets_test.py
"""
import subprocess
import sys
import tempfile
from pathlib import Path

from PIL import Image

# テスト対象を import
sys.path.insert(0, str(Path(__file__).parent))
import slice_character_sheets  # noqa: E402


def make_test_sheet(rows: int = 5, cols: int = 5, cell_size: int = 100) -> Image.Image:
    """
    テスト用 5×5 スプライトシートを生成する。
    各セルにユニークな色を塗って、分割後に内容検証できるようにする。
    """
    img = Image.new("RGB", (cols * cell_size, rows * cell_size), "white")
    for r in range(rows):
        for c in range(cols):
            # セルごとに (r, c) を識別できる色を塗る
            color = (r * 50, c * 50, 128)
            for y in range(r * cell_size, (r + 1) * cell_size):
                for x in range(c * cell_size, (c + 1) * cell_size):
                    img.putpixel((x, y), color)
    return img


def test_slice_sheet_creates_25_files() -> None:
    """5×5 の 500×500 シートを 25 枚に分割できる"""
    with tempfile.TemporaryDirectory() as tmp:
        tmp_path = Path(tmp)
        sheet_path = tmp_path / "test.png"
        out_dir = tmp_path / "out"
        img = make_test_sheet()
        img.save(sheet_path, "PNG")

        written = slice_character_sheets.slice_sheet(sheet_path, out_dir, cell_size=100)

        assert written == 25, f"expected 25 cells, got {written}"
        for r in range(5):
            for c in range(5):
                assert (out_dir / f"r{r}c{c}.png").exists(), f"missing r{r}c{c}.png"


def test_slice_sheet_preserves_cell_content() -> None:
    """分割後のセル内容が元シートの該当領域と一致する"""
    with tempfile.TemporaryDirectory() as tmp:
        tmp_path = Path(tmp)
        sheet_path = tmp_path / "test.png"
        out_dir = tmp_path / "out"
        img = make_test_sheet(cell_size=50)  # 250x250
        img.save(sheet_path, "PNG")

        slice_character_sheets.slice_sheet(sheet_path, out_dir, cell_size=50)

        # r2c2 (中央) は色 (100, 100, 128)
        cell_22 = Image.open(out_dir / "r2c2.png")
        # セル中央のピクセル
        center_color = cell_22.getpixel((25, 25))
        assert center_color == (100, 100, 128), f"r2c2 center: expected (100,100,128), got {center_color}"

        # r0c0 (左上) は色 (0, 0, 128)
        cell_00 = Image.open(out_dir / "r0c0.png")
        assert cell_00.getpixel((25, 25)) == (0, 0, 128)


def test_slice_sheet_skip_existing() -> None:
    """既存ファイルは上書きしない (--overwrite なし)"""
    with tempfile.TemporaryDirectory() as tmp:
        tmp_path = Path(tmp)
        sheet_path = tmp_path / "test.png"
        out_dir = tmp_path / "out"
        img = make_test_sheet(cell_size=20)
        img.save(sheet_path, "PNG")

        # 1 回目
        written1 = slice_character_sheets.slice_sheet(sheet_path, out_dir, cell_size=20)
        assert written1 == 25

        # 2 回目 (skip)
        written2 = slice_character_sheets.slice_sheet(sheet_path, out_dir, cell_size=20)
        assert written2 == 0, f"expected 0 (skip), got {written2}"


def test_slice_sheet_overwrite() -> None:
    """--overwrite で上書きできる"""
    with tempfile.TemporaryDirectory() as tmp:
        tmp_path = Path(tmp)
        sheet_path = tmp_path / "test.png"
        out_dir = tmp_path / "out"
        img = make_test_sheet(cell_size=20)
        img.save(sheet_path, "PNG")

        slice_character_sheets.slice_sheet(sheet_path, out_dir, cell_size=20)
        # 内容を変えて再書き出し
        img2 = make_test_sheet(cell_size=20)
        for x in range(img2.size[0]):
            for y in range(img2.size[1]):
                img2.putpixel((x, y), (0, 0, 0))
        img2.save(sheet_path, "PNG")

        written = slice_character_sheets.slice_sheet(
            sheet_path, out_dir, cell_size=20, overwrite=True
        )
        assert written == 25


def test_parse_sheet_pairs() -> None:
    """'A:path1,B:path2' パース"""
    pairs = slice_character_sheets.parse_sheet_pairs("A:foo.png,B:bar.png")
    assert len(pairs) == 2
    assert pairs[0] == ("A", Path("foo.png"))
    assert pairs[1] == ("B", Path("bar.png"))


def test_parse_sheet_pairs_invalid_sheet() -> None:
    """不正なシート名でエラー"""
    try:
        slice_character_sheets.parse_sheet_pairs("X:foo.png")
    except ValueError as e:
        assert "X" in str(e)
        return
    raise AssertionError("expected ValueError for invalid sheet name")


def test_main_single_file() -> None:
    """CLI 単一ファイルモード end-to-end"""
    with tempfile.TemporaryDirectory() as tmp:
        tmp_path = Path(tmp)
        sheet = tmp_path / "sheet.png"
        out = tmp_path / "out"
        make_test_sheet(cell_size=30).save(sheet, "PNG")

        result = subprocess.run(
            [
                sys.executable,
                str(Path(__file__).parent / "slice_character_sheets.py"),
                "--input", str(sheet),
                "--output", str(out),
                "--cell-size", "30",
            ],
            capture_output=True, text=True,
        )
        assert result.returncode == 0, f"stderr: {result.stderr}"
        # 25 枚出力
        assert (out / "r0c0.png").exists()
        assert (out / "r4c4.png").exists()


def test_main_multi_file() -> None:
    """CLI 複数ファイルモード end-to-end"""
    with tempfile.TemporaryDirectory() as tmp:
        tmp_path = Path(tmp)
        out = tmp_path / "char"
        for s in "ABC":
            sheet = tmp_path / f"sheet_{s}.png"
            make_test_sheet(cell_size=20).save(sheet, "PNG")

        result = subprocess.run(
            [
                sys.executable,
                str(Path(__file__).parent / "slice_character_sheets.py"),
                "--input-sheet", f"A:{tmp_path}/sheet_A.png,B:{tmp_path}/sheet_B.png,C:{tmp_path}/sheet_C.png",
                "--output-dir", str(out),
                "--cell-size", "20",
            ],
            capture_output=True, text=True,
        )
        assert result.returncode == 0, f"stderr: {result.stderr}"
        # 各シートディレクトリに 25 枚
        for s in "ABC":
            assert (out / s / "r0c0.png").exists()
            assert (out / s / "r4c4.png").exists()


def run_all() -> None:
    """すべてのテストを実行 (pytest がない環境用フォールバック)"""
    tests = [
        test_slice_sheet_creates_25_files,
        test_slice_sheet_preserves_cell_content,
        test_slice_sheet_skip_existing,
        test_slice_sheet_overwrite,
        test_parse_sheet_pairs,
        test_parse_sheet_pairs_invalid_sheet,
        test_main_single_file,
        test_main_multi_file,
    ]
    failed = 0
    for t in tests:
        try:
            t()
            print(f"  PASS: {t.__name__}")
        except Exception as e:
            print(f"  FAIL: {t.__name__}: {e}")
            failed += 1
    if failed:
        print(f"\n{failed} test(s) failed")
        sys.exit(1)
    print(f"\nAll {len(tests)} tests passed")


if __name__ == "__main__":
    run_all()
