# libcronet

**Chromium's `net/` stack as a standalone shared library with a plain C API.**

`libcronet` gives you the power of Chrome's networking engine—HTTP/3, QUIC, compression, and security—in a tiny, easy-to-embed package. It produces a single shared library (`libcronet.so` / `cronet.dll`) with a C interface that you can use from **C, C++, Go, Rust, Zig**, or any language with FFI support.

Under the hood, it patches [klzgrad/naiveproxy](https://github.com/klzgrad/naiveproxy)'s lightweight Chromium build to expose a stable, feature-rich API without the bloat of the full browser.

## Why use this?

*   **Identical TLS Fingerprint**: It *is* Chrome. Servers see you as a real browser.
*   **Modern Protocols**: HTTP/3 (QUIC) and HTTP/2 out of the box.
*   **Performance**: Brotli & Zstd decompression, aggressive caching, and connection pooling.
*   **Simplicity**: No JNI, no Java, no Objective-C. Just a header file and a shared lib.

## Features

| Feature | Status | Notes |
|---|---|---|
| **HTTP/1.1, H2, H3 (QUIC)** | ✅ | Automatic negotiation |
| **WebSockets** | ✅ | Fully supported (unlike NaiveProxy) |
| **TLS Configuration** | ✅ | Custom CAs, Min/Max Version, Insecure Skip |
| **Cookies** | ✅ | In-memory cookie jar |
| **Request Control** | ✅ | Timeouts, Priorities, Redirect limits |
| **Observability** | ✅ | Detailed timing metrics (DNS, SSL, TTFB) |
| **Compression** | ✅ | Brotli + Zstd |
| **Proxy Support** | ✅ | HTTP, SOCKS5, Custom rules |

## Language Support

### Go (Golang)

We provide first-class Go bindings in `go-cronet`. It implements `http.RoundTripper`, so it drops right into your existing code.

```go
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/uoosef403/naive_patch/go-cronet"
)

func main() {
	// Initialize the Cronet engine
	client := cronet.NewClient(
		cronet.WithUserAgent("MyApp/1.0"),
		cronet.WithQUIC(true),
	)
	
    // Use it like a standard http.Client
	req, _ := http.NewRequest("GET", "https://www.google.com", nil)
	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("Status: %s\nProtocol: %s\nBody len: %d\n", resp.Status, resp.Proto, len(body))
}
```

### C / C++

The API is defined in [`cronet_api.h`](patch_cronet/templates/cronet_api.h). Here's a simple synchronous example:

```c
#include "cronet_api.h"
#include <stdio.h>

int main(void) {
    // 1. Setup engine
    CronetEngineParams params = { .enable_quic = 1, .user_agent = "MyApp/1.0" };
    CronetEngine* engine = cronet_engine_create(&params);

    // 2. Create request
    CronetRequest* req = cronet_request_create(engine, "GET", "https://example.com", NULL, NULL);
    
    // 3. Start and wait (synchronous helper)
    cronet_request_start(req);
    cronet_request_wait(req, 5000); // 5s timeout

    // 4. Inspect result
    printf("Status: %d\n", cronet_request_get_status_code(req));
    printf("Protocol: %s\n", cronet_request_get_negotiated_protocol(req));

    // 5. Cleanup
    cronet_request_destroy(req);
    cronet_engine_destroy(engine);
    return 0;
}
```

## Quick Start (Docker)

The easiest way to build is using Docker. It handles all the complex Chromium dependencies for you.

```bash
# Build the builder image
docker build -t cronet .

# Extract artifacts
id=$(docker create cronet)
docker cp $id:/artifacts ./out
docker rm $id

ls out/
# libcronet.so  cronet.dll  cronet_api.h
```

### Cross-Compilation Targets

| Target OS | Command | Output |
|---|---|---|
| **Windows** (x64) | `docker build --build-arg TARGET_OS=win --build-arg TARGET_CPU=x64 ...` | `cronet.dll` |
| **Android** (arm64) | `docker build --build-arg TARGET_OS=android --build-arg TARGET_CPU=arm64 ...` | `libcronet.so` |
| **Linux** (arm64) | `docker build --build-arg TARGET_OS=linux --build-arg TARGET_CPU=arm64 ...` | `libcronet.so` |

## Manual Build (Advanced)

If you already have a NaiveProxy source tree:

```bash
# 1. Clone NaiveProxy
git clone https://github.com/klzgrad/naiveproxy.git
cd naiveproxy/src
./get-clang.sh

# 2. Patch it (from this repo's root)
python3 -m patch_cronet /path/to/naiveproxy/src

# 3. Build
cd /path/to/naiveproxy/src
./build_cronet.sh linux x64
```

## How it works

This project is a set of Python scripts that "patch" the NaiveProxy build system.

1.  **Injects C++ Shim**: Adds `cronet_engine.cc`, `cronet_request.cc`, etc., into the build tree. These files bridge the complex C++ internal Chromium APIs to a stable C ABI.
2.  **Modifies GN Files**: Updates `BUILD.gn` to produce a `shared_library` instead of an executable.
3.  **Fixes Toolchains**: Includes scripts to allow cross-compiling Windows DLLs from Linux (thanks to `xwin`).

## Credits

*   [klzgrad/naiveproxy](https://github.com/klzgrad/naiveproxy) for the stripped-down Chromium tree.
*   [The Chromium Project](https://www.chromium.org/) for the network stack.
*   [Jake-Shadle/xwin](https://github.com/Jake-Shadle/xwin) for the Windows cross-compilation magic.

## License

The C/Go bindings in this repo are **Public Domain**.
The resulting binary contains code from Chromium, subject to the [Chromium BSD-style license](https://chromium.googlesource.com/chromium/src/+/HEAD/LICENSE).
