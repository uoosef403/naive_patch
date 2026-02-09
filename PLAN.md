# Plan: Expand libcronet API + Comprehensive Go Library

## Context
The existing libcronet has 25 C functions covering basic HTTP requests and WebSockets, but lacks critical features for a production HTTP client: TLS bypass, timeouts, redirect control, cookie management, synchronous wait, H2/H3 protocol control, and response metadata. The goal is to:

1. Expand the C API to match Go's `http.Client` capabilities
2. Build `libcronet.so` for Linux
3. Create a **reusable Go library** with stable API/ABI suitable for use in other projects
4. Comprehensive unit + E2E test suites using **Caddy** as the test server for H2/H3/QUIC validation

---

## Phase 1: Expand C API (`patch_cronet/templates/`)

### 1a. Extended `CronetEngineParams` (append at end for ABI compat)

```c
// TLS
int         insecure_skip_verify;     // 1 = accept any cert
const char* custom_ca_pem;            // PEM CA bundle
int         tls_min_version;          // 0x0303=TLS1.2, 0x0304=TLS1.3
int         tls_max_version;

// Protocol control
int         enable_h2c;              // 1 = allow cleartext HTTP/2

// Cookies
int         enable_cookies;          // 1 = enable cookie jar
```

### 1b. New structs

- **`CronetRequestParams`**: `timeout_ms`, `connect_timeout_ms`, `max_redirects` (-1=default, 0=none), `priority`, `disable_cache`
- **`CronetTlsInfo`**: `cipher_suite`, `tls_version`, `negotiated_protocol`, `cert_chain_count`
- **`CronetTimingInfo`**: DNS, connect, SSL, send, first_byte, completion (int64_t ms)
- Constants: `CRONET_TLS_VERSION_*`, `CRONET_PRIORITY_*`

### 1c. 13 new exported functions (total: 38)

| Function | Purpose |
|----------|---------|
| `cronet_request_set_params` | Per-request timeout/redirects/priority |
| `cronet_request_wait` | Blocking wait via WaitableEvent |
| `cronet_request_get_url` | Final URL after redirects |
| `cronet_request_get_content_length` | Response Content-Length |
| `cronet_request_get_http_version` | "HTTP/1.1", "HTTP/2", "HTTP/3" |
| `cronet_request_get_tls_info` | TLS cipher, version, cert chain |
| `cronet_request_get_tls_cert_pem` | PEM cert at chain index |
| `cronet_request_get_timing` | DNS/connect/TLS/first-byte timing |
| `cronet_engine_set_cookie` | Set cookie programmatically |
| `cronet_engine_get_cookies` | Get cookies (caller frees) |
| `cronet_engine_clear_cookies` | Clear cookie jar |
| `cronet_error_to_string` | Net error code to string |
| `cronet_free_string` | Free library-allocated strings |

### 1d. C++ implementation (by file)

**`cronet_engine.h/cc`**:
- InsecureSkipVerify: `net::MockCertVerifier` with `set_default_result(net::OK)`
- Custom CA: `net::X509Certificate::CreateCertificateListFromBytes` + custom trust store
- TLS versions: `net::SSLConfig` via `net::StaticSSLConfigService`
- Cookie ops: `context()->cookie_store()` via PostTask + WaitableEvent
- Store `insecure_skip_verify_` flag for WebSocket

**`cronet_request.h/cc`**:
- Wait: `base::WaitableEvent completion_event_`, signal in `OnRequestComplete`
- Timeout: `base::OneShotTimer` on IO thread
- Redirects: counter + max check in `OnReceivedRedirect`
- TLS info: capture `request->ssl_info()` + PEM-encode cert chain
- Timing: `request->GetLoadTimingInfo()` in `OnRequestComplete`
- Priority: map to `net::RequestPriority`
- HTTP version: parse headers + ALPN for h2/h3 detection

**`cronet_websocket.cc`**: Check `engine_->insecure_skip_verify()` in `OnSSLCertificateError`

**`cronet_export.cc`**: 13 new `extern "C"` wrappers

No new source files - GN target unchanged.

---

## Phase 2: Build for Linux

1. Clean old Docker containers to free ~43GB: `docker container prune && docker image prune`
2. `docker build -t cronet-linux .` (defaults: TARGET_OS=linux, TARGET_CPU=x64)
3. Extract artifacts: `docker cp <id>:/artifacts/ ./artifacts/`
4. Verify: `nm -D libcronet.so | grep cronet_` shows 38 symbols

---

## Phase 3: Install Go

```bash
curl -fsSL https://go.dev/dl/go1.24.0.linux-amd64.tar.gz | sudo tar -C /usr/local -xzf -
```

---

## Phase 4: Go Library (`go-cronet/`)

### Architecture - reusable library with stable API

```
go-cronet/
  go.mod                    # module go-cronet
  go.sum

  # Public API
  cronet.go                 # Engine type, EngineConfig, NewEngine(), Close()
  transport.go              # Transport implementing http.RoundTripper
  client.go                 # Client - high-level HTTP client with options pattern
  request.go                # RequestConfig for per-request settings
  response.go               # Extended Response with TLS/Timing info
  websocket.go              # WebSocket - connect, send, receive, close
  errors.go                 # NetError type, error code constants
  cookies.go                # CookieJar wrapping engine cookie ops

  # Internal CGo bridge
  internal/capi/capi.go     # All #cgo declarations, C type wrappers
  internal/capi/callbacks.go # Callback trampolines + handle registry

  # Shared library + header
  lib/libcronet.so
  include/cronet_api.h

  # Tests
  cronet_test.go            # Unit tests
  transport_test.go         # http.RoundTripper tests
  client_test.go            # Client integration tests
  h2_test.go                # HTTP/2 specific tests
  h3_test.go                # HTTP/3 / QUIC specific tests
  tls_test.go               # TLS config tests
  cookie_test.go            # Cookie management tests
  websocket_test.go         # WebSocket tests
  timeout_test.go           # Timeout tests
  redirect_test.go          # Redirect behavior tests
  testutil_test.go          # Shared test helpers + Caddy server setup
```

### Key exported types (stable API)

```go
// Engine manages the underlying Chromium network stack.
type Engine struct { ... }

type EngineConfig struct {
    UserAgent          string
    EnableQUIC         bool        // HTTP/3 support
    EnableHTTP2        bool        // default true
    EnableBrotli       bool        // default true
    EnableZstd         bool
    ProxyURL           string
    CachePath          string
    CacheMaxBytes      int
    InsecureSkipVerify bool        // bypass TLS verification
    CustomCAPEM        string      // PEM CA bundle
    TLSMinVersion      uint16      // tls.VersionTLS12, etc.
    TLSMaxVersion      uint16
    EnableCookies      bool
}

// Transport implements http.RoundTripper.
type Transport struct {
    Engine         *Engine
    Timeout        time.Duration
    ConnectTimeout time.Duration
    MaxRedirects   int
    Priority       Priority
    DisableCache   bool
}

// Client is a high-level HTTP client (like Go's http.Client).
type Client struct { ... }
func NewClient(opts ...Option) (*Client, error)
func (c *Client) Do(req *http.Request) (*Response, error)
func (c *Client) Get(url string) (*Response, error)
func (c *Client) Post(url, contentType string, body io.Reader) (*Response, error)
func (c *Client) Close() error

// Response extends http.Response with Chromium-specific data.
type Response struct {
    *http.Response
    TLS    *TLSInfo
    Timing *TimingInfo
}

// Option pattern
func WithInsecureSkipVerify() Option
func WithTimeout(d time.Duration) Option
func WithQUIC(enable bool) Option
func WithHTTP2(enable bool) Option
func WithProxy(url string) Option
func WithMaxRedirects(n int) Option
func WithCustomCA(pem string) Option
func WithUserAgent(ua string) Option

// WebSocket
type WebSocket struct { ... }
func (e *Engine) DialWebSocket(url string, headers http.Header) (*WebSocket, error)
func (ws *WebSocket) ReadMessage() (data []byte, isBinary bool, err error)
func (ws *WebSocket) WriteMessage(data []byte, binary bool) error
func (ws *WebSocket) Close(code uint16, reason string) error

// CookieJar implements http.CookieJar using Chromium's CookieMonster.
type CookieJar struct { ... }
func (j *CookieJar) SetCookies(u *url.URL, cookies []*http.Cookie)
func (j *CookieJar) Cookies(u *url.URL) []*http.Cookie
```

### Callback bridging pattern
- Handle registry: `sync.Map[uint64]*requestState`
- Pass handle ID as `void* user_data` (uint64 -> uintptr -> unsafe.Pointer)
- Body streaming: `io.Pipe` - C callback writes to PipeWriter, Response.Body reads from PipeReader
- RoundTrip flow: start -> `on_response_started` signals headers-ready channel -> return `*http.Response` with pipe body -> `on_data_received` writes to pipe -> `on_completed` closes pipe

---

## Phase 5: Test Suite with Caddy

### Test server architecture (`testutil_test.go`)

Use **Caddy as a Go library** (`github.com/caddyserver/caddy/v2`) for a self-contained test server that supports H1, H2, and H3:

```go
// TestMain sets up a Caddy instance with:
// - HTTPS on port 14443 (auto self-signed cert) -> H1.1 + H2
// - HTTP/3 (QUIC) on same port 14443
// - HTTP on port 14080 (cleartext, for h2c testing)
// - Various handler routes for test scenarios
```

**Caddy test routes:**
| Route | Behavior |
|-------|----------|
| `/echo` | Echo back request headers + body as JSON |
| `/status/{code}` | Return specific status code |
| `/redirect/{n}` | Chain of n redirects ending at /echo |
| `/delay/{ms}` | Respond after delay (timeout testing) |
| `/cookies/set?k=v` | Set-Cookie header |
| `/cookies/get` | Return received cookies as JSON |
| `/large/{bytes}` | Return N bytes of data |
| `/tls-info` | Return TLS version/cipher as seen by server |
| `/h2-only` | Only available via HTTP/2 |
| `/ws/echo` | WebSocket echo endpoint |

### Test categories

**Unit tests** (`cronet_test.go`):
- Engine create/destroy, multiple engines, config validation
- Error code to string, version string, memory management

**HTTP/2 tests** (`h2_test.go`):
- `TestH2Negotiation` - Verify ALPN negotiates h2 with Caddy
- `TestH2Multiplexing` - Concurrent requests over single connection
- `TestH2ServerPush` - If applicable
- `TestH2LargeBody` - Streaming large response over h2
- `TestH2HeaderCompression` - HPACK verification via response size

**HTTP/3 / QUIC tests** (`h3_test.go`):
- `TestH3Negotiation` - Verify QUIC/h3 when enabled
- `TestH3Fallback` - Graceful fallback to h2 when h3 unavailable
- `TestH3LargeTransfer` - Large body over QUIC
- `TestH3VsH2Performance` - Compare both protocols
- `TestQUICDisabled` - Verify h2 used when QUIC disabled

**TLS tests** (`tls_test.go`):
- `TestInsecureSkipVerify` - Self-signed cert accepted
- `TestInsecureSkipVerifyDisabled` - Self-signed cert rejected by default
- `TestCustomCA` - Load Caddy's test CA, connect succeeds
- `TestTLSMinVersion13` - Set min TLS 1.3, verify
- `TestTLSInfo` - Read cipher suite, TLS version, cert chain
- `TestTLSCertChain` - Verify PEM cert extraction

**Timeout tests** (`timeout_test.go`):
- `TestRequestTimeout` - 100ms timeout against /delay/5000
- `TestConnectTimeout` - Against non-routable IP
- `TestNoTimeout` - Default (no timeout) works for slow response

**Redirect tests** (`redirect_test.go`):
- `TestFollowRedirects` - Default follows all
- `TestMaxRedirects3` - Stop after 3
- `TestDisableRedirects` - Get 301 directly
- `TestFinalURL` - URL after redirect chain

**Cookie tests** (`cookie_test.go`):
- `TestSetGetCookie` - Programmatic set, verify in request
- `TestServerSetCookie` - Server Set-Cookie, read via GetCookies
- `TestClearCookies` - Verify cleared
- `TestCookieJarInterface` - Use as http.CookieJar with http.Client

**Transport tests** (`transport_test.go`):
- `TestRoundTripper` - Verify interface compliance
- `TestWithStdHttpClient` - `&http.Client{Transport: t}` works
- `TestConcurrent100` - 100 parallel requests
- `TestAllHTTPMethods` - GET, POST, PUT, DELETE, PATCH, HEAD, OPTIONS

**WebSocket tests** (`websocket_test.go`):
- `TestWSConnect` - Connect via Caddy WSS
- `TestWSTextMessage` - Send/receive text
- `TestWSBinaryMessage` - Send/receive binary
- `TestWSLargeMessage` - Large frame reassembly
- `TestWSClose` - Clean close handshake

**Client E2E tests** (`client_test.go`):
- `TestClientGetJSON` - GET + parse JSON
- `TestClientPostForm` - POST form data
- `TestClientPostJSON` - POST JSON body
- `TestClientStreaming` - Stream large response body
- `TestClientWithAllOptions` - Every option set
- `TestClientH2WithCaddy` - Verify h2 against Caddy
- `TestClientH3WithCaddy` - Verify h3 against Caddy

---

## Phase 6: Implementation Order

1. **C API expansion** - All template files
2. **Docker build** - Clean space, build, extract .so
3. **Install Go** on host
4. **Go library core** - capi bridge, Engine, Transport
5. **Test infrastructure** - Caddy server setup, test routes
6. **Go library features** - Client, WebSocket, CookieJar, Response
7. **Unit tests** - All test files
8. **E2E tests** - Full protocol tests with Caddy
9. **Verification** - `go test -v -race ./...` all pass

## Verification
1. `nm -D libcronet.so | grep cronet_` shows all 38 symbols
2. `go test -v -race ./...` - all ~50+ tests pass
3. H2 test confirms `alpn = "h2"` via Caddy
4. H3 test confirms QUIC negotiation via Caddy
5. TLS tests verify InsecureSkipVerify works against self-signed Caddy
