# libcronet

A shared library that exposes Chromium's network stack through a C API. The
repository also includes Go bindings in [`go-cronet/`](go-cronet/).

## Uses

- Add HTTP requests to applications written in C or languages that call C APIs.
- Use Chromium networking from Go applications.
- Open WebSocket connections and exchange messages.
- Inspect request timing, the negotiated protocol, and TLS connection details.
- Configure proxies, cookies, and cache settings for an HTTP client.

The API supports HTTP/2 and QUIC configuration, request headers and bodies,
timeouts, redirects, and cancellation. See
[`cronet_api.h`](patch_cronet/templates/cronet_api.h) for the C interface.
The library does not include a browser UI or JavaScript engine.

## Build with Docker

The default build targets Linux x64. It downloads the upstream source and build
dependencies, so it needs network access and significant disk space.

```sh
docker build -t cronet .
id=$(docker create cronet)
docker cp "$id:/artifacts" ./out
docker rm "$id"
```

The output directory contains the library and `cronet_api.h`. To select a
different target, set `TARGET_OS` and `TARGET_CPU`:

```sh
docker build --build-arg TARGET_OS=win --build-arg TARGET_CPU=x64 -t cronet-win .
docker build --build-arg TARGET_OS=android --build-arg TARGET_CPU=arm64 -t cronet-android .
```

The build scripts define targets for Linux, Windows, and Android. iOS is not
supported. See [`build_cronet.sh`](patch_cronet/templates/build_cronet.sh) for
target options and the [Dockerfile](Dockerfile) for the source and build steps.

## Source

- `patch_cronet/` applies the build patches and contains the C API shim.
- `go-cronet/` contains Go bindings and tests.

The patches depend on Chromium internal APIs. Changes to upstream sources can
require changes to this project. Review the licenses and distribution
requirements of upstream components when distributing a build.
