"""Entry point for patch_cronet: patches NaiveProxy tree to build libcronet.so."""
import sys
from pathlib import Path
from . import patcher


def main():
    if len(sys.argv) > 1:
        src_dir = Path(sys.argv[1])
    else:
        src_dir = Path("/workspace/src")

    if not src_dir.is_dir():
        print(f"Error: source directory '{src_dir}' does not exist", file=sys.stderr)
        sys.exit(1)

    patcher.run(src_dir)


main()
