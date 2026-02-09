"""Orchestrates all patches to produce a cronet-building NaiveProxy tree."""
import shutil
import stat
from pathlib import Path

from . import gn_patches

# Map of template filename -> destination path relative to src_dir
_TEMPLATE_DESTINATIONS = {
    "build_cronet.sh": "build_cronet.sh",
    "cronet_api.h": "net/tools/cronet/cronet_api.h",
    "cronet_engine.h": "net/tools/cronet/cronet_engine.h",
    "cronet_engine.cc": "net/tools/cronet/cronet_engine.cc",
    "cronet_request.h": "net/tools/cronet/cronet_request.h",
    "cronet_request.cc": "net/tools/cronet/cronet_request.cc",
    "cronet_websocket.h": "net/tools/cronet/cronet_websocket.h",
    "cronet_websocket.cc": "net/tools/cronet/cronet_websocket.cc",
    "cronet_export.cc": "net/tools/cronet/cronet_export.cc",
}


def run(src_dir: Path) -> None:
    """Apply all patches and copy templates into the source tree."""
    print(f"Patching NaiveProxy tree at {src_dir} for cronet build...")

    # 1. Patch BUILD.gn files
    print("Patching BUILD.gn files...")
    gn_patches.patch_root_build_gn(src_dir)
    gn_patches.patch_net_build_gn(src_dir)
    gn_patches.patch_perfetto_gni(src_dir)
    gn_patches.patch_visual_studio_version_gni(src_dir)
    gn_patches.patch_setup_toolchain_py(src_dir)
    gn_patches.patch_base_build_gn(src_dir)

    # 2. Copy template files to their destinations
    templates_dir = Path(__file__).parent / "templates"
    cronet_dir = src_dir / "net" / "tools" / "cronet"
    cronet_dir.mkdir(parents=True, exist_ok=True)

    print("Copying template files...")
    for template_name, dest_rel in _TEMPLATE_DESTINATIONS.items():
        src_file = templates_dir / template_name
        dest_file = src_dir / dest_rel
        dest_file.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(src_file, dest_file)
        print(f"  {template_name} -> {dest_rel}")

    # 3. Make build script executable
    build_script = src_dir / "build_cronet.sh"
    build_script.chmod(build_script.stat().st_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)

    # 4. Copy the public header to out/ so it's easy to find after build
    include_dir = src_dir / "out"
    include_dir.mkdir(parents=True, exist_ok=True)
    shutil.copy2(templates_dir / "cronet_api.h", include_dir / "cronet_api.h")
    print(f"  cronet_api.h -> out/cronet_api.h (public header)")

    print("Done. Run ./build_cronet.sh [os] [cpu] to build.")
    print("  Linux:   ./build_cronet.sh linux x64|arm64|arm|x86")
    print("  Windows: ./build_cronet.sh win x64|arm64  (auto-fetches SDK via xwin)")
    print("  Android: ./build_cronet.sh android arm64|arm|x64|x86  (auto-fetches NDK)")
