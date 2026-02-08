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
    && rm -rf /var/lib/apt/lists/*

# For Android builds: mount system images
RUN apt-get update && apt-get install -y \
    sudo \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /workspace

RUN git clone https://github.com/klzgrad/naiveproxy.git .

WORKDIR /workspace/src

# Build GN using g++ to avoid Clang/libstdc++ compatibility issues
RUN git clone https://gn.googlesource.com/gn tmp/gn && \
    cd tmp/gn && \
    CXX=g++ CC=gcc python3 build/gen.py && \
    ninja -C out && \
    mkdir -p ../../gn/out && \
    cp out/gn ../../gn/out/ && \
    cd ../.. && \
    rm -rf tmp/gn

# Download the Clang toolchain
RUN ./get-clang.sh

# Copy and apply the cronet patches
COPY patch_cronet/ /opt/patch_cronet/
ENV PYTHONPATH=/opt
RUN python3 -m patch_cronet /workspace/src

# ── Build target ─────────────────────────────────────────────────────────────
# Override with --build-arg:
#   docker build --build-arg TARGET_OS=win --build-arg TARGET_CPU=x64 .
#   docker build --build-arg TARGET_OS=android --build-arg TARGET_CPU=arm64 .
#   docker build --build-arg TARGET_OS=linux --build-arg TARGET_CPU=arm64 .
ARG TARGET_OS=linux
ARG TARGET_CPU=x64

# For Android: fetch NDK + system image before building
RUN if [ "$TARGET_OS" = "android" ]; then \
        EXTRA_FLAGS="target_os=\"android\" target_cpu=\"${TARGET_CPU}\"" ./get-clang.sh && \
        EXTRA_FLAGS="target_os=\"android\" target_cpu=\"${TARGET_CPU}\"" ./get-android-sys.sh || true; \
    fi

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
