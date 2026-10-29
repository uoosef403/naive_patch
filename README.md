# libcronet

Build Chromium's network stack as a shared library with a C API. This project
patches the [NaiveProxy](https://github.com/klzgrad/naiveproxy) source tree to
build the library instead of the proxy executable. It also includes Go bindings
in [`go-cronet/`](go-cronet/).

The API includes HTTP requests, WebSockets, proxy settings, cookies, and cache
settings. See [`cronet_api.h`](patch_cronet/templates/cronet_api.h) for the C
interface. The library does not include a browser UI or JavaScript engine.

## Build with Docker

The default build targets Linux x64. It downloads NaiveProxy and its build
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
target options.

## Build without Docker

Clone NaiveProxy and download its Clang toolchain. Then apply the patches from
this repository and run the generated build script:

```sh
git clone https://github.com/klzgrad/naiveproxy.git
cd naiveproxy/src
./get-clang.sh
cd /path/to/naive_patch
python3 -m patch_cronet /path/to/naiveproxy/src
cd /path/to/naiveproxy/src
./build_cronet.sh linux x64
```

The patches depend on Chromium internal APIs. Changes to NaiveProxy or Chromium
can require changes to this project.

## Source

- `patch_cronet/` applies the build patches and contains the C API shim.
- `go-cronet/` contains Go bindings and tests.

Chromium and NaiveProxy have their own license and distribution requirements.
Review those requirements when distributing a build.
