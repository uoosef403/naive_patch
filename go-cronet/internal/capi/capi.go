// Package capi provides low-level CGo bindings to libcronet.
package capi

// Platform-specific library linking is handled by files in the platform/ subdirectory
// with appropriate build tags (linux/amd64, android/arm64, windows/amd64, etc.)

/*
#cgo CFLAGS: -I${SRCDIR}/../../include
#include "cronet_api.h"
#include <stdlib.h>
*/
import "C"
import "unsafe"

// Re-export C types for use by the main package.
type Engine = C.CronetEngine
type Request = C.CronetRequest
type WebSocket = C.CronetWebSocket
type EngineParams = C.CronetEngineParams
type RequestParams = C.CronetRequestParams
type RequestCallbacks = C.CronetRequestCallbacks
type WebSocketCallbacks = C.CronetWebSocketCallbacks
type TlsInfo = C.CronetTlsInfo
type TimingInfo = C.CronetTimingInfo

// Constants
const (
	TLSVersionDefault = C.CRONET_TLS_VERSION_DEFAULT
	TLSVersionTLS10   = C.CRONET_TLS_VERSION_TLS_1_0
	TLSVersionTLS11   = C.CRONET_TLS_VERSION_TLS_1_1
	TLSVersionTLS12   = C.CRONET_TLS_VERSION_TLS_1_2
	TLSVersionTLS13   = C.CRONET_TLS_VERSION_TLS_1_3

	PriorityDefault = C.CRONET_PRIORITY_DEFAULT
	PriorityIdle    = C.CRONET_PRIORITY_IDLE
	PriorityLowest  = C.CRONET_PRIORITY_LOWEST
	PriorityLow     = C.CRONET_PRIORITY_LOW
	PriorityMedium  = C.CRONET_PRIORITY_MEDIUM
	PriorityHighest = C.CRONET_PRIORITY_HIGHEST
)

// --- Version ---

func Version() string {
	return C.GoString(C.cronet_version())
}

func UserAgentString() string {
	return C.GoString(C.cronet_user_agent_string())
}

// --- Engine ---

func EngineCreate(params *EngineParams) *Engine {
	return C.cronet_engine_create(params)
}

func EngineDestroy(engine *Engine) {
	C.cronet_engine_destroy(engine)
}

func EngineSetCookie(engine *Engine, url, cookieLine string) {
	curl := C.CString(url)
	ccookie := C.CString(cookieLine)
	defer C.free(unsafe.Pointer(curl))
	defer C.free(unsafe.Pointer(ccookie))
	C.cronet_engine_set_cookie(engine, curl, ccookie)
}

func EngineGetCookies(engine *Engine, url string) string {
	curl := C.CString(url)
	defer C.free(unsafe.Pointer(curl))
	cstr := C.cronet_engine_get_cookies(engine, curl)
	if cstr == nil {
		return ""
	}
	result := C.GoString(cstr)
	C.cronet_free_string(cstr)
	return result
}

func EngineClearCookies(engine *Engine) {
	C.cronet_engine_clear_cookies(engine)
}

// --- Request ---

func RequestCreate(engine *Engine, method, url string, callbacks *RequestCallbacks, userData unsafe.Pointer) *Request {
	cmethod := C.CString(method)
	curl := C.CString(url)
	defer C.free(unsafe.Pointer(cmethod))
	defer C.free(unsafe.Pointer(curl))
	return C.cronet_request_create(engine, cmethod, curl, callbacks, userData)
}

func RequestAddHeader(req *Request, name, value string) {
	cname := C.CString(name)
	cvalue := C.CString(value)
	defer C.free(unsafe.Pointer(cname))
	defer C.free(unsafe.Pointer(cvalue))
	C.cronet_request_add_header(req, cname, cvalue)
}

func RequestSetBody(req *Request, contentType string, data []byte) {
	cct := C.CString(contentType)
	defer C.free(unsafe.Pointer(cct))
	if len(data) > 0 {
		C.cronet_request_set_body(req, cct, (*C.char)(unsafe.Pointer(&data[0])), C.size_t(len(data)))
	} else {
		C.cronet_request_set_body(req, cct, nil, 0)
	}
}

func RequestSetParams(req *Request, params *RequestParams) {
	C.cronet_request_set_params(req, params)
}

func RequestStart(req *Request) {
	C.cronet_request_start(req)
}

func RequestCancel(req *Request) {
	C.cronet_request_cancel(req)
}

func RequestDestroy(req *Request) {
	C.cronet_request_destroy(req)
}

func RequestWait(req *Request, timeoutMs int) int {
	return int(C.cronet_request_wait(req, C.int(timeoutMs)))
}

func RequestGetStatusCode(req *Request) int {
	return int(C.cronet_request_get_status_code(req))
}

func RequestGetHeader(req *Request, name string) string {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	cval := C.cronet_request_get_header(req, cname)
	if cval == nil {
		return ""
	}
	return C.GoString(cval)
}

func RequestGetHeaderCount(req *Request) int {
	return int(C.cronet_request_get_header_count(req))
}

func RequestGetHeaderName(req *Request, index int) string {
	cval := C.cronet_request_get_header_name(req, C.int(index))
	if cval == nil {
		return ""
	}
	return C.GoString(cval)
}

func RequestGetHeaderValue(req *Request, index int) string {
	cval := C.cronet_request_get_header_value(req, C.int(index))
	if cval == nil {
		return ""
	}
	return C.GoString(cval)
}

func RequestGetNegotiatedProtocol(req *Request) string {
	cval := C.cronet_request_get_negotiated_protocol(req)
	if cval == nil {
		return ""
	}
	return C.GoString(cval)
}

func RequestGetURL(req *Request) string {
	cval := C.cronet_request_get_url(req)
	if cval == nil {
		return ""
	}
	return C.GoString(cval)
}

func RequestGetContentLength(req *Request) int64 {
	return int64(C.cronet_request_get_content_length(req))
}

func RequestGetHTTPVersion(req *Request) string {
	cval := C.cronet_request_get_http_version(req)
	if cval == nil {
		return ""
	}
	return C.GoString(cval)
}

func RequestGetTLSInfo(req *Request) (cipher uint16, version uint16, proto string, certCount int, ok bool) {
	var info TlsInfo
	ret := C.cronet_request_get_tls_info(req, &info)
	if ret != 0 {
		return 0, 0, "", 0, false
	}
	proto = ""
	if info.negotiated_protocol != nil {
		proto = C.GoString(info.negotiated_protocol)
	}
	return uint16(info.cipher_suite), uint16(info.tls_version), proto, int(info.cert_chain_count), true
}

func RequestGetTLSCertPEM(req *Request, index int) string {
	cval := C.cronet_request_get_tls_cert_pem(req, C.int(index))
	if cval == nil {
		return ""
	}
	return C.GoString(cval)
}

func RequestGetTiming(req *Request) (info TimingInfo, ok bool) {
	ret := C.cronet_request_get_timing(req, &info)
	return info, ret == 0
}

// --- WebSocket ---

func WebSocketCreate(engine *Engine, callbacks *WebSocketCallbacks, userData unsafe.Pointer) *WebSocket {
	return C.cronet_websocket_create(engine, callbacks, userData)
}

func WebSocketAddHeader(ws *WebSocket, name, value string) {
	cname := C.CString(name)
	cvalue := C.CString(value)
	defer C.free(unsafe.Pointer(cname))
	defer C.free(unsafe.Pointer(cvalue))
	C.cronet_websocket_add_header(ws, cname, cvalue)
}

func WebSocketConnect(ws *WebSocket, url string, subprotocols []string) {
	curl := C.CString(url)
	defer C.free(unsafe.Pointer(curl))

	if len(subprotocols) == 0 {
		C.cronet_websocket_connect(ws, curl, nil, 0)
		return
	}

	cprotos := make([]*C.char, len(subprotocols))
	for i, p := range subprotocols {
		cprotos[i] = C.CString(p)
	}
	defer func() {
		for _, cp := range cprotos {
			C.free(unsafe.Pointer(cp))
		}
	}()

	C.cronet_websocket_connect(ws, curl, &cprotos[0], C.int(len(subprotocols)))
}

func WebSocketSend(ws *WebSocket, data []byte, isBinary bool) {
	bin := C.int(0)
	if isBinary {
		bin = 1
	}
	if len(data) > 0 {
		C.cronet_websocket_send(ws, (*C.char)(unsafe.Pointer(&data[0])), C.size_t(len(data)), bin)
	} else {
		C.cronet_websocket_send(ws, nil, 0, bin)
	}
}

func WebSocketClose(ws *WebSocket, code uint16, reason string) {
	creason := C.CString(reason)
	defer C.free(unsafe.Pointer(creason))
	C.cronet_websocket_close(ws, C.uint16_t(code), creason)
}

func WebSocketDestroy(ws *WebSocket) {
	C.cronet_websocket_destroy(ws)
}

// --- Utility ---

func ErrorToString(netError int) string {
	cval := C.cronet_error_to_string(C.int(netError))
	if cval == nil {
		return ""
	}
	return C.GoString(cval)
}

// Helper: create a C EngineParams from Go config fields.
func NewEngineParams(userAgent string, enableQUIC, enableHTTP2, enableBrotli, enableZstd bool,
	proxyURL, cachePath string, cacheMaxBytes int,
	insecureSkipVerify bool, customCAPEM string,
	tlsMin, tlsMax int, enableCookies bool) (EngineParams, []*C.char) {

	// Track C strings for cleanup
	var cstrs []*C.char
	toCStr := func(s string) *C.char {
		if s == "" {
			return nil
		}
		cs := C.CString(s)
		cstrs = append(cstrs, cs)
		return cs
	}

	boolToInt := func(b bool) C.int {
		if b {
			return 1
		}
		return 0
	}

	p := EngineParams{
		user_agent:           toCStr(userAgent),
		enable_quic:          boolToInt(enableQUIC),
		enable_http2:         boolToInt(enableHTTP2),
		enable_brotli:        boolToInt(enableBrotli),
		enable_zstd:          boolToInt(enableZstd),
		proxy_url:            toCStr(proxyURL),
		cache_path:           toCStr(cachePath),
		cache_max_bytes:      C.int(cacheMaxBytes),
		insecure_skip_verify: boolToInt(insecureSkipVerify),
		custom_ca_pem:        toCStr(customCAPEM),
		tls_min_version:      C.int(tlsMin),
		tls_max_version:      C.int(tlsMax),
		enable_cookies:       boolToInt(enableCookies),
	}
	return p, cstrs
}

// FreeStrings frees a slice of C strings.
func FreeStrings(cstrs []*C.char) {
	for _, cs := range cstrs {
		if cs != nil {
			C.free(unsafe.Pointer(cs))
		}
	}
}

// NewRequestParams creates C request params.
func NewRequestParams(timeoutMs, connectTimeoutMs, maxRedirects, priority int, disableCache bool) RequestParams {
	dc := C.int(0)
	if disableCache {
		dc = 1
	}
	return RequestParams{
		timeout_ms:         C.int(timeoutMs),
		connect_timeout_ms: C.int(connectTimeoutMs),
		max_redirects:      C.int(maxRedirects),
		priority:           C.int(priority),
		disable_cache:      dc,
	}
}
