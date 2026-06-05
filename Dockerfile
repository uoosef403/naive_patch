FROM ubuntu:22.04 AS base

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y \
    git \
    python3 \
    ninja-build \
    pkg-config \
    ccache \
    bubblewrap \
    wget \
    curl \
    xz-utils \
    unzip \
    libnss3-dev \
    build-essential \
    python-is-python3 \
    sudo \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /workspace

RUN git clone https://github.com/klzgrad/naiveproxy.git .

WORKDIR /workspace/src

# Build GN using g++ to avoid Clang/libstdc++ compatibility issues
# -Wno-comment: newer GCC errors on "//" inside comments (GN source has these)
RUN git clone https://gn.googlesource.com/gn tmp/gn && \
    cd tmp/gn && \
    CXX="g++ -Wno-comment" CC=gcc python3 build/gen.py && \
    ninja -C out && \
    mkdir -p ../../gn/out && \
    cp out/gn ../../gn/out/ && \
    cd ../.. && \
    rm -rf tmp/gn

# Download the Clang toolchain
RUN ./get-clang.sh

# ── Pre-download Windows SDK + PGO (cached before patches) ─────────────────
# These downloads are ~1.4 GB combined and rarely change; caching them avoids
# re-downloading on every patch_cronet/ edit.
ARG TARGET_OS=linux
ARG TARGET_CPU=x64

# xwin: Windows SDK + CRT (only for win targets)
RUN if [ "$TARGET_OS" = "win" ]; then \
        XWIN_DIR="third_party/xwin" && \
        XWIN_SDK="$XWIN_DIR/sdk" && \
        mkdir -p "$XWIN_DIR" && \
        XWIN_VERSION="0.6.5" && \
        curl -L "https://github.com/Jake-Shadle/xwin/releases/download/${XWIN_VERSION}/xwin-${XWIN_VERSION}-x86_64-unknown-linux-musl.tar.gz" \
            | tar xzf - --strip-components=1 -C "$XWIN_DIR" && \
        "$XWIN_DIR/xwin" --accept-license splat --output "$XWIN_SDK"; \
    fi

# PGO profile (fetched for all platforms)
RUN if grep -q 'chrome_pgo_phase' build/config/compiler/pgo/pgo.gni 2>/dev/null; then \
        case "$TARGET_OS" in \
            win) case "$TARGET_CPU" in \
                    arm64) _PGO=win-arm64;; x64) _PGO=win64;; *) _PGO=win32;; \
                 esac;; \
            linux) _PGO=linux;; \
            android) case "$TARGET_CPU" in \
                        arm64) _PGO=android-arm64;; *) _PGO=android-arm32;; \
                     esac;; \
            *) _PGO="";; \
        esac && \
        if [ "$_PGO" ] && [ -f "chrome/build/${_PGO}.pgo.txt" ]; then \
            PGO_PATH=$(cat "chrome/build/${_PGO}.pgo.txt") && \
            mkdir -p chrome/build/pgo_profiles && \
            curl --limit-rate 10M -Lo "chrome/build/pgo_profiles/$PGO_PATH" \
                "https://storage.googleapis.com/chromium-optimization-profiles/pgo_profiles/$PGO_PATH"; \
        fi; \
    fi

# For Android: fetch NDK + system image before building
RUN if [ "$TARGET_OS" = "android" ]; then \
        EXTRA_FLAGS="target_os=\"android\" target_cpu=\"${TARGET_CPU}\"" ./get-clang.sh && \
        EXTRA_FLAGS="target_os=\"android\" target_cpu=\"${TARGET_CPU}\"" ./get-android-sys.sh || true; \
    fi

# ── Apply patches (changes here only rebuild from this point) ──────────────
COPY patch_cronet/ /opt/patch_cronet/
ENV PYTHONPATH=/opt
RUN python3 -m patch_cronet /workspace/src

# ── Build ──────────────────────────────────────────────────────────────────
RUN ./build_cronet.sh ${TARGET_OS} ${TARGET_CPU}

# ── Collect artifacts ────────────────────────────────────────────────────────
RUN mkdir -p /artifacts && \
    cp out/cronet_api.h /artifacts/ && \
    (cp out/Cronet-*/libcronet.so /artifacts/ 2>/dev/null || true) && \
    (cp out/Cronet-*/cronet.dll /artifacts/ 2>/dev/null || true)

# ── Extraction stage (optional: docker build --target=artifacts) ─────────────
FROM scratch AS artifacts
COPY --from=base /artifacts/ /

# Default: drop into shell for inspection
FROM base
CMD ["/bin/bash"]
