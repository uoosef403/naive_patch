# libcronet

Chromium's `net/` stack as a standalone shared library with a plain C API.
Produces `libcronet.so` (Linux/Android) or `cronet.dll` (Windows) with an
identical TLS fingerprint to Google Chrome.

Built on top of [klzgrad/naiveproxy](https://github.com/klzgrad/naiveproxy),
which maintains a stripped-down Chromium source tree that actually compiles
in reasonable time (~15 min vs hours for full Chrome). This project patches
NaiveProxy's build to produce a shared library instead of the `naive` proxy
executable, re-enables WebSockets (NaiveProxy disables them), and removes the
proxy-specific padding protocol.

## What you get

A single shared library exporting 25 C functions. The header is
[`cronet_api.h`](patch_cronet/templates/cronet_api.h). Usable from C, Zig
(`@cImport`), Rust (`bindgen`), Go (`cgo`), or anything with an FFI.

| Feature | Status |
|---|---|
| HTTP/1.1, HTTP/2, HTTP/3 (QUIC) | Yes |
| WebSocket | Yes |
| TLS fingerprint | Identical to Chrome |
| Brotli + Zstd decompression | Yes |
| Cookies (in-memory) | Yes |
| Proxy (HTTP/SOCKS) | Configurable |
| Disk cache | Optional |

What it does **not** include: browser UI, rendering, JavaScript, DevTools,
NaiveProxy's padding protocol, or any of the other 90% of Chromium.

## Quick start

```
docker build -t cronet .
id=$(docker create cronet)
docker cp $id:/artifacts ./out
docker rm $id
ls out/   # libcronet.so  cronet_api.h
```

For a specific platform:

```
docker build --build-arg TARGET_OS=win --build-arg TARGET_CPU=x64 -t cronet-win .
docker build --build-arg TARGET_OS=android --build-arg TARGET_CPU=arm64 -t cronet-android .
docker build --build-arg TARGET_OS=linux --build-arg TARGET_CPU=arm64 -t cronet-linux-arm64 .
```

## Supported targets

| OS | Architectures | Output | Notes |
|---|---|---|---|
| `linux` | `x64`, `arm64`, `arm`, `x86` | `libcronet.so` | Sysroot built from Debian snapshot |
| `win` | `x64`, `arm64` | `cronet.dll` | Cross-compiled from Linux via [xwin](https://github.com/Jake-Shadle/xwin) |
| `android` | `arm64`, `arm`, `x64`, `x86` | `libcronet.so` | NDK fetched automatically, min SDK 24 |

iOS is not supported. Apple requires Xcode on macOS to link Mach-O binaries.
There is no cross-compilation path from Linux.

## Build without Docker

If you already have NaiveProxy cloned:

```
git clone https://github.com/klzgrad/naiveproxy.git
cd naiveproxy/src
./get-clang.sh

# From this repo's root:
python3 -m patch_cronet /path/to/naiveproxy/src
cd /path/to/naiveproxy/src
./build_cronet.sh linux x64
```

## Usage example (C)

```c
#include "cronet_api.h"
#include <stdio.h>

static int on_data(CronetRequest* req, const char* data, size_t len, void* ctx) {
    fwrite(data, 1, len, stdout);
    return 0;
}

static void on_done(CronetRequest* req, int err, void* ctx) {
    printf("\n--- done (net_error=%d) ---\n", err);
}

int main(void) {
    CronetEngineParams p = { .enable_quic = 1, .enable_http2 = 1 };
    CronetEngine* engine = cronet_engine_create(&p);

    CronetRequestCallbacks cb = { .on_data_received = on_data, .on_completed = on_done };
    CronetRequest* req = cronet_request_create(engine, "GET", "https://example.com", &cb, NULL);
    cronet_request_start(req);

    sleep(3);  // callbacks fire on the IO thread

    cronet_request_destroy(req);
    cronet_engine_destroy(engine);
}
```

Compile: `cc example.c -L. -lcronet -o example`

## CI / GitHub Actions

Push this repo to GitHub and it builds everything automatically. The workflow
(`.github/workflows/build.yml`) produces 6 artifacts:

- `cronet-linux-x64`, `cronet-linux-arm64`
- `cronet-win-x64`, `cronet-win-arm64`
- `cronet-android-arm64`, `cronet-android-arm`

On push to `main`, a GitHub Release is created with `.tar.gz` bundles.

## How it works

`patch_cronet/` is a Python module that does three things:

1. **Patches `BUILD.gn`** -- adds a `shared_library("cronet")` target to `net/BUILD.gn`
   and registers it in the root `gn_all` group.
2. **Copies C++ sources** into `net/tools/cronet/` -- a thin shim layer:
   - `cronet_engine.cc` -- spins up a `base::Thread` with IO pump, builds a
     `URLRequestContext` via the builder API.
   - `cronet_request.cc` -- `URLRequest::Delegate` impl with async read loop.
   - `cronet_websocket.cc` -- `WebSocketEventInterface` impl with frame reassembly.
   - `cronet_export.cc` -- `extern "C"` wrappers that delegate to the above.
3. **Installs `build_cronet.sh`** -- GN args corrected from NaiveProxy's defaults
   (`enable_websockets=true`, proxy-specific flags removed).

The C++ shim is ~800 lines total. It talks directly to Chromium's internal APIs
(`URLRequestContextBuilder`, `WebSocketChannel`, etc.) -- there is no
abstraction layer or wrapper library in between.

## Updating NaiveProxy

When NaiveProxy bumps its Chromium version:

1. `cd naiveproxy && git pull`
2. `python3 -m patch_cronet src/`
3. `cd src && ./build_cronet.sh`

If it compiles, you're done. If not, the compiler error will point at the
exact line in `cronet_*.cc` where a Chromium internal API changed (a renamed
method, an added parameter, etc.). These are typically 1-3 line fixes.

The patching is idempotent -- safe to run repeatedly.

## Project structure

```
.
├── Dockerfile                          # Autonomous build: clone, patch, compile
├── .github/workflows/build.yml         # CI for 6 platform/arch combos
└── patch_cronet/
    ├── __main__.py                     # python3 -m patch_cronet [src_dir]
    ├── patcher.py                      # Orchestrator
    ├── gn_patches.py                   # BUILD.gn modifications
    └── templates/
        ├── build_cronet.sh             # GN args + ninja invocation
        ├── cronet_api.h                # Public C header (25 functions)
        ├── cronet_engine.{h,cc}        # IO thread + URLRequestContext
        ├── cronet_request.{h,cc}       # HTTP request (URLRequest::Delegate)
        ├── cronet_websocket.{h,cc}     # WebSocket (WebSocketEventInterface)
        └── cronet_export.cc            # extern "C" function bodies
```

## Acknowledgements

This project exists because of:

- [klzgrad/naiveproxy](https://github.com/klzgrad/naiveproxy) -- for
  maintaining a buildable subset of Chromium's network stack and doing
  the hard work of tracking upstream.
- [The Chromium project](https://www.chromium.org/) -- for the actual
  network stack, BoringSSL, QUIC implementation, and everything else
  under the hood.
- [Jake-Shadle/xwin](https://github.com/Jake-Shadle/xwin) -- for making
  Windows cross-compilation from Linux possible without distributing the
  Windows SDK.

## License

The C API shim (`patch_cronet/templates/`) is public domain (do whatever you want).
The resulting binary links against Chromium code, which is under the
[Chromium BSD-style license](https://chromium.googlesource.com/chromium/src/+/HEAD/LICENSE).
