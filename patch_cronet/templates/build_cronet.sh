#!/bin/bash
set -euo pipefail

# Build script for libcronet.so / cronet.dll / libcronet.dylib
# Based on NaiveProxy's build.sh with corrected GN args.
#
# Usage:
#   ./build_cronet.sh                          # linux x64 (auto-detect)
#   ./build_cronet.sh linux arm64              # linux aarch64
#   ./build_cronet.sh win x64                  # windows 64-bit (cross-compile)
#   ./build_cronet.sh win arm64                # windows arm64
#   ./build_cronet.sh android arm64            # android aarch64
#   ./build_cronet.sh android arm              # android armv7

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

TARGET_OS="${1:-linux}"
TARGET_CPU="${2:-x64}"

out="out/Cronet-${TARGET_OS}-${TARGET_CPU}"

# ── Base GN args (all platforms) ─────────────────────────────────────────────
flags="
is_cronet_build=true
is_official_build=true
is_debug=false
is_component_build=false

is_clang=true
use_sysroot=false
fatal_linker_warnings=false
treat_warnings_as_errors=false

target_os=\"${TARGET_OS}\"
target_cpu=\"${TARGET_CPU}\"

# Re-enable WebSocket support (NaiveProxy disables this)
enable_websockets=true

# Disable NaiveProxy-specific proxy features
enable_bracketed_proxy_uris=false
enable_quic_proxy_support=false

# Disable features not needed for a library
use_kerberos=false
disable_file_support=true
enable_mdns=false
enable_reporting=false
include_transport_security_state_preload_list=false
enable_device_bound_sessions=false

# Match NaiveProxy's stripped-down build
use_udev=false
use_aura=false
use_ozone=false
use_gio=false
use_glib=false
use_nss_certs=false
enable_backup_ref_ptr_support=false
enable_dangling_raw_ptr_checks=false

# Optimisation
exclude_unwind_tables=true
enable_resource_allowlist_generation=false
symbol_level=0
"

# ── Platform-specific flags ──────────────────────────────────────────────────
case "$TARGET_OS" in
    linux)
        # For cross-compilation, build a sysroot
        if [ "$TARGET_CPU" != "x64" ] || [ "${FORCE_SYSROOT:-}" = "1" ]; then
            if [ -f "./get-sysroot.sh" ]; then
                echo "=== Setting up sysroot for linux-${TARGET_CPU} ==="
                export EXTRA_FLAGS="target_cpu=\"${TARGET_CPU}\""
                . ./get-sysroot.sh
                if [ "$SYSROOT_ARCH" ] && [ ! -d "./$WITH_SYSROOT/lib" ]; then
                    ./build/linux/sysroot_scripts/sysroot_creator.py build "$SYSROOT_ARCH"
                fi
                if [ "$WITH_SYSROOT" ]; then
                    flags+="
target_sysroot=\"//$WITH_SYSROOT\""
                fi
                unset EXTRA_FLAGS
            fi
        fi
        # LLVM cross-compile workaround
        if [ "$TARGET_CPU" = "x64" ]; then
            flags+=$'\nuse_cfi_icall=false'
        fi
        ;;

    win)
        # ── Windows cross-compilation from Linux ──
        # Downloads Windows SDK + MSVC CRT via xwin, then passes explicit
        # paths to GN so vs_toolchain.py auto-detection is bypassed.
        XWIN_DIR="$SCRIPT_DIR/third_party/xwin"
        XWIN_SDK="$XWIN_DIR/sdk"

        if [ ! -d "$XWIN_SDK/Include" ]; then
            echo "=== Downloading Windows SDK + CRT via xwin ==="
            XWIN_BIN="$XWIN_DIR/xwin"
            if [ ! -f "$XWIN_BIN" ]; then
                mkdir -p "$XWIN_DIR"
                XWIN_VERSION="0.6.5"
                curl -L "https://github.com/Jake-Shadle/xwin/releases/download/${XWIN_VERSION}/xwin-${XWIN_VERSION}-x86_64-unknown-linux-musl.tar.gz" \
                    | tar xzf - --strip-components=1 -C "$XWIN_DIR"
            fi
            "$XWIN_BIN" --accept-license splat --output "$XWIN_SDK"
            echo "=== Windows SDK downloaded to $XWIN_SDK ==="
        fi

        # Map xwin's layout to what Chromium expects:
        #   xwin splats to: sdk/Include/, sdk/Lib/, sdk/crt/include/, sdk/crt/lib/
        # Chromium GN needs:
        #   visual_studio_path  -> contains VC/Tools/MSVC/<ver>/
        #   windows_sdk_path    -> contains Include/<sdk_ver>/ and Lib/<sdk_ver>/
        XWIN_VS="$XWIN_SDK/vs"
        if [ ! -d "$XWIN_VS/VC/Tools/MSVC" ]; then
            echo "=== Arranging SDK layout for Chromium ==="
            mkdir -p "$XWIN_VS/VC/Tools/MSVC/14.40.0"
            # CRT headers + libs
            if [ -d "$XWIN_SDK/crt/include" ]; then
                ln -sfn "$XWIN_SDK/crt/include" "$XWIN_VS/VC/Tools/MSVC/14.40.0/include"
                mkdir -p "$XWIN_VS/VC/Tools/MSVC/14.40.0/lib"
                for arch_dir in "$XWIN_SDK/crt/lib/"*; do
                    if [ -d "$arch_dir" ]; then
                        ln -sfn "$arch_dir" "$XWIN_VS/VC/Tools/MSVC/14.40.0/lib/$(basename "$arch_dir")"
                    fi
                done
            fi
        fi

        # Read the SDK version from directory listing
        WIN_SDK_VER=""
        if [ -d "$XWIN_SDK/Include" ]; then
            WIN_SDK_VER=$(ls "$XWIN_SDK/Include/" | grep '^10\.' | head -1)
        fi
        if [ -z "$WIN_SDK_VER" ]; then
            WIN_SDK_VER="10.0.26100.0"
        fi

        export DEPOT_TOOLS_WIN_TOOLCHAIN=0
        flags+="
visual_studio_path=\"${XWIN_VS}\"
visual_studio_version=\"2022\"
windows_sdk_path=\"${XWIN_SDK}\"
windows_sdk_version=\"${WIN_SDK_VER}\"
wdk_path=\"\"
use_lld=true"
        ;;

    android)
        # ── Android cross-compilation ──
        # NDK is fetched by get-clang.sh when target_os=android.
        # If not present, fetch it now.
        if [ ! -d "third_party/android_toolchain/ndk" ]; then
            echo "=== Fetching Android NDK ==="
            export EXTRA_FLAGS="target_os=\"android\" target_cpu=\"${TARGET_CPU}\""
            ./get-clang.sh
            unset EXTRA_FLAGS
        fi

        # Android system image (provides system libraries for linking)
        if [ -f "./get-android-sys.sh" ]; then
            echo "=== Fetching Android system image ==="
            export EXTRA_FLAGS="target_os=\"android\" target_cpu=\"${TARGET_CPU}\""
            ./get-android-sys.sh
            # Set sysroot from system image
            . ./get-sysroot.sh
            if [ "$WITH_ANDROID_IMG" ] && [ -d "out/sysroot-build/android/$WITH_ANDROID_IMG" ]; then
                flags+="
android_ndk_root=\"//third_party/android_toolchain/ndk\""
            fi
            unset EXTRA_FLAGS
        fi

        flags+="
default_min_sdk_version=24
is_high_end_android=true"
        ;;
esac

# ── PGO profiles ─────────────────────────────────────────────────────────────
if grep -q 'chrome_pgo_phase' build/config/compiler/pgo/pgo.gni 2>/dev/null; then
    # Fetch PGO profile if not present
    case "$TARGET_OS" in
        win)
            case "$TARGET_CPU" in
                arm64) _PGO=win-arm64;;
                x64)   _PGO=win64;;
                *)     _PGO=win32;;
            esac;;
        linux) _PGO=linux;;
        android)
            case "$TARGET_CPU" in
                arm64) _PGO=android-arm64;;
                *)     _PGO=android-arm32;;
            esac;;
        *) _PGO="";;
    esac
    if [ "$_PGO" ] && [ -f "chrome/build/${_PGO}.pgo.txt" ]; then
        PGO_PATH=$(cat "chrome/build/${_PGO}.pgo.txt")
        if [ ! -f "chrome/build/pgo_profiles/$PGO_PATH" ]; then
            echo "=== Fetching PGO profile for ${_PGO} ==="
            mkdir -p chrome/build/pgo_profiles
            curl --limit-rate 10M -Lo "chrome/build/pgo_profiles/$PGO_PATH" \
                "https://storage.googleapis.com/chromium-optimization-profiles/pgo_profiles/$PGO_PATH"
        fi
    fi
    flags+=$'\nchrome_pgo_phase=2'
fi

# ── ccache ───────────────────────────────────────────────────────────────────
if command -v ccache &>/dev/null; then
    export CCACHE_SLOPPINESS=time_macros
    export CCACHE_BASEDIR="$PWD"
    export CCACHE_CPP2=yes
    flags+=$'\ncc_wrapper="ccache"'
fi

# ── Extra flags from environment ─────────────────────────────────────────────
if [ -n "${BUILD_EXTRA_FLAGS:-}" ]; then
    flags+="$BUILD_EXTRA_FLAGS"
fi

export DEPOT_TOOLS_WIN_TOOLCHAIN="${DEPOT_TOOLS_WIN_TOOLCHAIN:-0}"

echo "=== Target: ${TARGET_OS} ${TARGET_CPU} ==="
echo "=== Output: ${out} ==="
echo "=== Generating build files ==="
./gn/out/gn gen "$out" --args="$flags"

echo "=== Building cronet ==="
ninja -C "$out" cronet

echo ""
echo "=== Build complete ==="
case "$TARGET_OS" in
    win)
        ls -lh "$out/cronet.dll" 2>/dev/null || echo "Warning: cronet.dll not found"
        ;;
    android)
        ls -lh "$out/libcronet.so" 2>/dev/null || echo "Warning: libcronet.so not found"
        ;;
    *)
        ls -lh "$out/libcronet.so" 2>/dev/null || echo "Warning: libcronet.so not found"
        echo ""
        echo "Exported symbols:"
        nm -D "$out/libcronet.so" 2>/dev/null | grep ' T cronet_' || true
        ;;
esac
