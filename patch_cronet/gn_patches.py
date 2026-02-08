"""Patches BUILD.gn files to add the cronet shared_library target."""
from pathlib import Path


def patch_root_build_gn(src_dir: Path) -> None:
    """Insert '//net:cronet' into group("gn_all") deps in the root BUILD.gn."""
    build_gn = src_dir / "BUILD.gn"
    text = build_gn.read_text()

    marker = '"//net:cronet"'
    if marker in text:
        print("  root BUILD.gn already patched, skipping")
        return

    # The existing deps list contains "//net", add our target after it
    old = '"//net",'
    if old not in text:
        # Try without trailing comma
        old = '"//net"'
        new = '"//net",\n    "//net:cronet",'
    else:
        new = '"//net",\n    "//net:cronet",'

    text = text.replace(old, new, 1)
    build_gn.write_text(text)
    print("  patched root BUILD.gn: added //net:cronet to gn_all deps")


_CRONET_TARGET = '''
shared_library("cronet") {
  sources = [
    "tools/cronet/cronet_engine.cc",
    "tools/cronet/cronet_engine.h",
    "tools/cronet/cronet_request.cc",
    "tools/cronet/cronet_request.h",
    "tools/cronet/cronet_websocket.cc",
    "tools/cronet/cronet_websocket.h",
    "tools/cronet/cronet_export.cc",
  ]

  deps = [
    ":net",
    "//base",
    "//components/version_info",
    "//url",
  ]

  defines = [ "CRONET_BUILDING_SHARED" ]
}
'''


def patch_net_build_gn(src_dir: Path) -> None:
    """Append the shared_library("cronet") target to net/BUILD.gn."""
    build_gn = src_dir / "net" / "BUILD.gn"
    text = build_gn.read_text()

    if 'shared_library("cronet")' in text:
        print("  net/BUILD.gn already patched, skipping")
        return

    text += _CRONET_TARGET
    build_gn.write_text(text)
    print("  patched net/BUILD.gn: appended shared_library(\"cronet\") target")
