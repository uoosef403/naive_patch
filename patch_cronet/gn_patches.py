"""Patches BUILD.gn files to add the cronet shared_library target."""
from pathlib import Path


def patch_visual_studio_version_gni(src_dir: Path) -> None:
    """Fix visual_studio_runtime_dirs type for cross-compilation.

    Chromium's visual_studio_version.gni sets visual_studio_runtime_dirs = []
    (list) when a custom visual_studio_path is provided, but
    win_toolchain_data.gni passes it to exec_script() which requires a string.
    Move the variable into declare_args() so it can be overridden from build
    args, and remove the hard [] assignment in the else branch.
    """
    gni = src_dir / "build" / "config" / "win" / "visual_studio_version.gni"
    if not gni.exists():
        print("  visual_studio_version.gni not found, skipping")
        return

    text = gni.read_text()
    marker = "# cronet-patched: visual_studio_runtime_dirs"
    if marker in text:
        print("  visual_studio_version.gni already patched, skipping")
        return

    # 1. Add visual_studio_runtime_dirs = "" to declare_args() block
    #    (insert right before the closing brace of the first declare_args)
    da_close = 'windows_sdk_version = ""'
    if da_close in text:
        text = text.replace(
            da_close,
            da_close + '\n\n  ' + marker + '\n  visual_studio_runtime_dirs = ""',
            1,
        )

    # 2. Remove the hard assignment visual_studio_runtime_dirs = [] in else
    text = text.replace("  visual_studio_runtime_dirs = []\n", "")

    gni.write_text(text)
    print("  patched visual_studio_version.gni: "
          "visual_studio_runtime_dirs is now an overridable string arg")


def patch_setup_toolchain_py(src_dir: Path) -> None:
    """Patch setup_toolchain.py to support cross-compilation from Linux.

    When DEPOT_TOOLS_WIN_TOOLCHAIN=0 on a non-Windows host, Chromium's
    setup_toolchain.py tries to detect Visual Studio and run vcvarsall.bat,
    which both fail on Linux.  Insert a code path that constructs the INCLUDE,
    LIB, and PATH environment synthetically from the SDK/CRT directory layout
    that xwin + our remapping has prepared.
    """
    py = src_dir / "build" / "toolchain" / "win" / "setup_toolchain.py"
    if not py.exists():
        print("  setup_toolchain.py not found, skipping")
        return

    text = py.read_text()
    marker = "# cronet-patched: cross-compile"
    if marker in text:
        print("  setup_toolchain.py already patched, skipping")
        return

    # --- 1. Add _SyntheticCrossCompileEnv helper after _ExtractImportantEnvironment ---
    helper = '''

{marker}
def _FindArchDir(base, cpu):
  """Return base/cpu if it exists, otherwise fall back to any available subdir."""
  exact = os.path.join(base, cpu)
  if os.path.isdir(exact):
    return exact
  if os.path.isdir(base):
    for d in sorted(os.listdir(base)):
      p = os.path.join(base, d)
      if os.path.isdir(p):
        return p
  return None

def _SyntheticCrossCompileEnv(cpu, toolchain_root, sdk_dir):
  """Build INCLUDE/LIB/PATH for cross-compiling from a non-Windows host."""
  include, lib, path_dirs = [], [], []

  # MSVC CRT (provided via xwin crt/ remapped into vs/VC/Tools/MSVC/<ver>/)
  msvc_base = os.path.join(toolchain_root, 'VC', 'Tools', 'MSVC')
  if os.path.isdir(msvc_base):
    msvc_ver = sorted(os.listdir(msvc_base))[-1]
    msvc = os.path.join(msvc_base, msvc_ver)
    d = os.path.join(msvc, 'include')
    if os.path.isdir(d):
      include.append(d)
    arch_lib = _FindArchDir(os.path.join(msvc, 'lib'), cpu)
    if arch_lib:
      lib.append(arch_lib)
    d = os.path.join(msvc, 'bin')
    if os.path.isdir(d):
      path_dirs.append(d)

  # Windows SDK (Include/<ver>/ucrt etc, Lib/<ver>/ucrt/<cpu> etc)
  sdk_inc = os.path.join(sdk_dir, 'Include')
  if os.path.isdir(sdk_inc):
    versions = sorted(
        [d for d in os.listdir(sdk_inc) if d.startswith('10.')],
        reverse=True)
    if versions:
      v = versions[0]
      for sub in ('ucrt', 'shared', 'um', 'winrt', 'cppwinrt'):
        p = os.path.join(sdk_inc, v, sub)
        if os.path.isdir(p):
          include.append(p)
      for sub in ('ucrt', 'um'):
        arch_lib = _FindArchDir(os.path.join(sdk_dir, 'Lib', v, sub), cpu)
        if arch_lib:
          lib.append(arch_lib)

  env = {{}}
  env['INCLUDE'] = ';'.join(include)
  env['LIB'] = ';'.join(lib)
  env['PATH'] = os.pathsep.join(path_dirs) + os.pathsep + os.environ.get('PATH', '')
  return env

'''.format(marker=marker)

    anchor = "def _DetectVisualStudioPath():"
    if anchor not in text:
        print("  setup_toolchain.py: anchor not found, skipping")
        return
    text = text.replace(anchor, helper + anchor)

    # --- 2. In _LoadToolchainEnv else branch, intercept non-Windows hosts ---
    old_else = """\
  else:
    if 'GYP_MSVS_OVERRIDE_PATH' not in os.environ:
      os.environ['GYP_MSVS_OVERRIDE_PATH'] = _DetectVisualStudioPath()"""

    new_else = """\
  else:
    if sys.platform not in ('win32', 'cygwin'):
      return _SyntheticCrossCompileEnv(cpu, toolchain_root, sdk_dir)
    if 'GYP_MSVS_OVERRIDE_PATH' not in os.environ:
      os.environ['GYP_MSVS_OVERRIDE_PATH'] = _DetectVisualStudioPath()"""

    if old_else not in text:
        print("  setup_toolchain.py: else-branch anchor not found, skipping")
        return
    text = text.replace(old_else, new_else)

    # --- 3. Make cl.exe lookup optional for cross-compilation ---
    old_cl = "vc_bin_dir = FindFileInEnvList(env, 'PATH', os.pathsep, 'cl.exe')"
    new_cl = ("vc_bin_dir = FindFileInEnvList(env, 'PATH', os.pathsep, "
              "'cl.exe', optional=True)")
    if old_cl in text:
        text = text.replace(old_cl, new_cl)

    py.write_text(text)
    print("  patched setup_toolchain.py: added cross-compilation support")


def patch_perfetto_gni(src_dir: Path) -> None:
    """Force perfetto into embedder mode.

    NaiveProxy sets build_with_chromium = false.  This makes perfetto enter
    standalone mode and try to import //gn/standalone/*.gni files that don't
    exist.  We force perfetto_build_with_embedder = true so it uses the
    Chromium embedder path instead.
    """
    gni = src_dir / "third_party" / "perfetto" / "gn" / "perfetto.gni"
    if not gni.exists():
        print("  perfetto.gni not found, skipping")
        return

    text = gni.read_text()
    marker = "# cronet-patched: force embedder mode"
    if marker in text:
        print("  perfetto.gni already patched, skipping")
        return

    old = "if (!defined(perfetto_build_with_embedder)) {"
    new = (marker + "\nperfetto_build_with_embedder = true\n"
           "if (false && !defined(perfetto_build_with_embedder)) {")
    if old not in text:
        print("  perfetto.gni: anchor not found, skipping")
        return

    text = text.replace(old, new, 1)
    gni.write_text(text)
    print("  patched perfetto.gni: forced perfetto_build_with_embedder = true")


def patch_base_build_gn(src_dir: Path) -> None:
    """Remove ATL source files from base/BUILD.gn for cross-compilation.

    ATL (Active Template Library) headers are part of Visual Studio but not
    available via xwin.  No code in base/, net/, or cronet actually uses ATL,
    so we can safely remove these three source entries from the is_win block.
    """
    build_gn = src_dir / "base" / "BUILD.gn"
    if not build_gn.exists():
        print("  base/BUILD.gn not found, skipping")
        return

    text = build_gn.read_text()
    marker = "# cronet-patched: removed ATL sources"
    if marker in text:
        print("  base/BUILD.gn already patched (ATL), skipping")
        return

    atl_files = [
        '    "win/atl.h",\n',
        '    "win/atl_throw.cc",\n',
        '    "win/atl_throw.h",\n',
    ]

    changed = False
    for atl_line in atl_files:
        if atl_line in text:
            text = text.replace(atl_line, "", 1)
            changed = True

    if changed:
        # Add marker comment at the top
        text = marker + "\n" + text
        build_gn.write_text(text)
        print("  patched base/BUILD.gn: removed ATL source files "
              "(atl.h, atl_throw.cc, atl_throw.h)")
    else:
        print("  base/BUILD.gn: ATL source lines not found, skipping")


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
    "//base/i18n/streaming_utf8_validator.cc",
    "//base/i18n/utf8_validator_tables.cc",
  ]

  deps = [
    ":net",
    "//base",
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
