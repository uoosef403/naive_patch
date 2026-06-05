#!/bin/bash
set -eo pipefail

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
enable_disk_cache_sql_backend=false

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
use_thin_lto=false
use_cfi_icall=false
is_cfi=false
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
            # Portable x86-64 flags (avoid illegal instruction on older CPUs)
            flags+=$'\ncronet_extra_cflags=\"-march=x86-64 -mtune=generic -mno-avx512f -mno-avx512dq -mno-avx512bw -mno-avx512vl -mno-avx512cd -mno-avx512vbmi -mno-vaes -mno-vpclmulqdq\"'
            flags+=$'\ncronet_extra_cxxflags=\"-march=x86-64 -mtune=generic -mno-avx512f -mno-avx512dq -mno-avx512bw -mno-avx512vl -mno-avx512cd -mno-avx512vbmi -mno-vaes -mno-vpclmulqdq\"'
            flags+=$'\ncronet_extra_ldflags=\"-march=x86-64 -mno-avx512f -mno-avx512dq -mno-avx512bw -mno-avx512vl -mno-avx512cd -mno-avx512vbmi -mno-vaes -mno-vpclmulqdq\"'
        fi
        ;;

    win)
        # ── Windows cross-compilation from Linux ──
        # Downloads Windows SDK + MSVC CRT via xwin, then remaps the flat
        # output into the versioned layout that Chromium's GN expects.
        XWIN_DIR="$SCRIPT_DIR/third_party/xwin"
        XWIN_SDK="$XWIN_DIR/sdk"
        XWIN_BIN="$XWIN_DIR/xwin"

        # Download xwin binary if needed
        if [ ! -f "$XWIN_BIN" ]; then
            echo "=== Downloading xwin ==="
            mkdir -p "$XWIN_DIR"
            XWIN_VERSION="0.6.5"
            curl -L "https://github.com/Jake-Shadle/xwin/releases/download/${XWIN_VERSION}/xwin-${XWIN_VERSION}-x86_64-unknown-linux-musl.tar.gz" \
                | tar xzf - --strip-components=1 -C "$XWIN_DIR"
        fi

        # Download Windows SDK + CRT if needed
        if [ ! -d "$XWIN_SDK/crt" ]; then
            echo "=== Downloading Windows SDK + CRT via xwin ==="
            "$XWIN_BIN" --accept-license splat --output "$XWIN_SDK"
            echo "=== Windows SDK downloaded to $XWIN_SDK ==="
        fi

        # xwin 0.6.x outputs a flat layout:
        #   $XWIN_SDK/crt/{include, lib/{x86_64,aarch64,...}}
        #   $XWIN_SDK/sdk/include/{ucrt,um,shared,winrt,cppwinrt}
        #   $XWIN_SDK/sdk/lib/{ucrt,um}/{x86_64,aarch64,...}
        #
        # Chromium's GN expects a Visual Studio-style versioned layout:
        #   windows_sdk_path/Include/<version>/{ucrt,um,shared,...}
        #   windows_sdk_path/Lib/<version>/{ucrt,um}/{x64,arm64,...}
        #   visual_studio_path/VC/Tools/MSVC/<ver>/include
        #   visual_studio_path/VC/Tools/MSVC/<ver>/lib/{x64,arm64,...}

        WIN_SDK_VER="10.0.26100.0"
        MSVC_VER="14.40.0"

        # Helper: map xwin arch names to MSVC arch names
        _msvc_arch() {
            case "$1" in
                x86_64)  echo "x64" ;;
                aarch64) echo "arm64" ;;
                *)       echo "$1" ;;
            esac
        }

        # Remap SDK headers into versioned Include/ layout
        if [ -d "$XWIN_SDK/sdk/include" ] && [ ! -d "$XWIN_SDK/Include/$WIN_SDK_VER" ]; then
            echo "=== Remapping SDK include layout for Chromium ==="
            mkdir -p "$XWIN_SDK/Include/$WIN_SDK_VER"
            for d in "$XWIN_SDK/sdk/include/"*; do
                [ -d "$d" ] && ln -sfn "$d" "$XWIN_SDK/Include/$WIN_SDK_VER/$(basename "$d")"
            done
        fi

        # Remap SDK libs into versioned Lib/ layout with MSVC arch names
        if [ -d "$XWIN_SDK/sdk/lib" ] && [ ! -d "$XWIN_SDK/Lib/$WIN_SDK_VER" ]; then
            echo "=== Remapping SDK lib layout for Chromium ==="
            mkdir -p "$XWIN_SDK/Lib/$WIN_SDK_VER"
            for d in "$XWIN_SDK/sdk/lib/"*; do
                if [ -d "$d" ]; then
                    cat_name=$(basename "$d")  # e.g. "ucrt", "um"
                    mkdir -p "$XWIN_SDK/Lib/$WIN_SDK_VER/$cat_name"
                    for arch_d in "$d/"*; do
                        if [ -d "$arch_d" ]; then
                            ln -sfn "$arch_d" "$XWIN_SDK/Lib/$WIN_SDK_VER/$cat_name/$(_msvc_arch "$(basename "$arch_d")")"
                        fi
                    done
                fi
            done
        fi

        # Set up VS layout for CRT
        XWIN_VS="$XWIN_SDK/vs"
        if [ ! -d "$XWIN_VS/VC/Tools/MSVC/$MSVC_VER" ]; then
            echo "=== Arranging CRT layout for Chromium ==="
            mkdir -p "$XWIN_VS/VC/Tools/MSVC/$MSVC_VER"
            if [ -d "$XWIN_SDK/crt/include" ]; then
                ln -sfn "$XWIN_SDK/crt/include" "$XWIN_VS/VC/Tools/MSVC/$MSVC_VER/include"
                mkdir -p "$XWIN_VS/VC/Tools/MSVC/$MSVC_VER/lib"
                for arch_dir in "$XWIN_SDK/crt/lib/"*; do
                    if [ -d "$arch_dir" ]; then
                        ln -sfn "$arch_dir" "$XWIN_VS/VC/Tools/MSVC/$MSVC_VER/lib/$(_msvc_arch "$(basename "$arch_dir")")"
                    fi
                done
            fi
        fi

        # ── Case-sensitivity fix ──
        # xwin on Linux may store headers in lowercase (e.g. winsock2.h) but
        # Chromium includes use original Windows case (e.g. Winsock2.h).
        # Linux filesystems are case-sensitive, so we need to create symlinks
        # for the original-case names.  We use xwin's --include-debug-libs
        # is not needed, but we DO need symlinks.
        #
        # Check if headers are lowercase and create case-correcting symlinks.
        _fix_sdk_case() {
            local sdk_inc="$1"
            if [ ! -d "$sdk_inc" ]; then return; fi

            echo "=== Fixing SDK header case for case-sensitive filesystem ==="

            # Common Windows SDK headers that Chromium includes with specific case
            # Map: lowercase_name -> OriginalCase
            declare -A CASE_MAP=(
                # um/ headers
                ["winsock2.h"]="Winsock2.h"
                ["mswsock.h"]="Mswsock.h"
                ["ws2tcpip.h"]="Ws2tcpip.h"
                ["windows.h"]="Windows.h"
                ["wincrypt.h"]="WinCrypt.h"
                ["iphlpapi.h"]="Iphlpapi.h"
                ["nb30.h"]="Nb30.h"
                ["ncrypt.h"]="NCrypt.h"
                ["ntsecapi.h"]="NtSecAPI.h"
                ["rpc.h"]="Rpc.h"
                ["winspool.h"]="WinSpool.h"
                ["dbghelp.h"]="DbgHelp.h"
                ["shlobj.h"]="ShlObj.h"
                ["winhttp.h"]="WinHttp.h"
                ["wininet.h"]="WinInet.h"
                ["setupapi.h"]="SetupAPI.h"
                ["shellapi.h"]="ShellApi.h"
                ["shlwapi.h"]="Shlwapi.h"
                ["winscard.h"]="WinSCard.h"
                ["mmsystem.h"]="MMSystem.h"
                ["powrprof.h"]="PowrProf.h"
                ["psapi.h"]="Psapi.h"
                ["lmcons.h"]="LMCons.h"
                ["wtsapi32.h"]="WtsApi32.h"
                ["pdh.h"]="Pdh.h"
                ["mstcpip.h"]="MSTcpIP.h"
                ["msxml6.h"]="MsXml6.h"
                ["accctrl.h"]="AccCtrl.h"
                ["aclapi.h"]="AclAPI.h"
                ["commdlg.h"]="CommDlg.h"
                ["commoncontrols.h"]="CommonControls.h"
                ["dbt.h"]="Dbt.h"
                ["dwmapi.h"]="DwmApi.h"
                ["ipexport.h"]="IPExport.h"
                ["icmpapi.h"]="IcmpAPI.h"
                ["lm.h"]="Lm.h"
                ["softpub.h"]="SoftPub.h"
                ["mscat.h"]="MsCat.h"
                ["iprtrmib.h"]="IpRtrMib.h"
                ["iptypes.h"]="IPTypes.h"
                ["tcpmib.h"]="TcpMib.h"
                ["udpmib.h"]="UdpMib.h"
                ["ifmib.h"]="IfMib.h"
                ["ipmib.h"]="IpMib.h"
                ["netioapi.h"]="NetioApi.h"
                ["mprapi.h"]="MprApi.h"
                ["wlanapi.h"]="WlanAPI.h"
                # shared/ headers
                ["guiddef.h"]="GuidDef.h"
                ["wmistr.h"]="WmiStr.h"
                ["evntrace.h"]="EvnTrace.h"
                ["stralign.h"]="StrAlign.h"
                ["inaddr.h"]="InAddr.h"
                ["in6addr.h"]="In6Addr.h"
                ["devpropdef.h"]="DevPropDef.h"
                ["tcpestats.h"]="TcpEStats.h"
            )

            # For each include subdirectory (um, shared, ucrt, etc.)
            for inc_sub in "$sdk_inc"/*/; do
                [ -d "$inc_sub" ] || continue
                for lower_name in "${!CASE_MAP[@]}"; do
                    orig_case="${CASE_MAP[$lower_name]}"
                    lower_file="$inc_sub/$lower_name"
                    orig_file="$inc_sub/$orig_case"
                    # If lowercase exists but original case does not, create symlink
                    if [ -f "$lower_file" ] && [ ! -e "$orig_file" ]; then
                        ln -s "$lower_name" "$orig_file"
                    fi
                    # If original case exists but lowercase does not, create symlink
                    if [ -f "$orig_file" ] && [ ! -e "$lower_file" ]; then
                        ln -s "$orig_case" "$lower_file"
                    fi
                done
            done

            echo "=== SDK header case fix complete ==="
        }

        _fix_sdk_case "$XWIN_SDK/sdk/include"

        # Also verify key headers exist (debugging aid)
        echo "=== SDK include verification ==="
        for h in "um/Winsock2.h" "um/Windows.h" "shared/GuidDef.h" "ucrt/stdlib.h"; do
            if [ -f "$XWIN_SDK/sdk/include/$h" ]; then
                echo "  OK: $h"
            else
                echo "  MISSING: $h"
            fi
        done

        export DEPOT_TOOLS_WIN_TOOLCHAIN=0
        flags+="
visual_studio_path=\"${XWIN_VS}\"
visual_studio_version=\"2022\"
windows_sdk_path=\"${XWIN_SDK}\"
windows_sdk_version=\"${WIN_SDK_VER}\"
wdk_path=\"${XWIN_SDK}\"
visual_studio_runtime_dirs=\"${XWIN_VS}/VC/Tools/MSVC/$MSVC_VER/lib/x64\"
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
            curl -Lo "chrome/build/pgo_profiles/$PGO_PATH" \
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

NINJA_JOBS="${NINJA_JOBS:-$(nproc)}"
echo "=== Building cronet (${NINJA_JOBS} parallel jobs) ==="
ninja -j "$NINJA_JOBS" -C "$out" cronet

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
