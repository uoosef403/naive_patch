package capi

/*
#include "cronet_api.h"

// Forward declarations for Go callback trampolines
extern int goOnResponseStarted(CronetRequest* req, int status_code,
                                const char* status_text, void* user_data);
extern int goOnDataReceived(CronetRequest* req, const char* data,
                            size_t length, void* user_data);
extern void goOnCompleted(CronetRequest* req, int net_error, void* user_data);
extern int goOnRedirect(CronetRequest* req, int status_code,
                        const char* new_url, void* user_data);

extern void goWSOnConnected(CronetWebSocket* ws, const char* subprotocol,
                             void* user_data);
extern void goWSOnMessage(CronetWebSocket* ws, const char* data,
                           size_t length, int is_binary, void* user_data);
extern void goWSOnClosed(CronetWebSocket* ws, int was_clean, uint16_t code,
                          const char* reason, void* user_data);
extern void goWSOnError(CronetWebSocket* ws, int net_error,
                         const char* message, void* user_data);

// Create callbacks struct with Go trampolines
static CronetRequestCallbacks makeGoRequestCallbacks() {
    CronetRequestCallbacks cb = {0};
    cb.on_response_started = goOnResponseStarted;
    cb.on_data_received = goOnDataReceived;
    cb.on_completed = goOnCompleted;
    cb.on_redirect = goOnRedirect;
    return cb;
}

static CronetWebSocketCallbacks makeGoWSCallbacks() {
    CronetWebSocketCallbacks cb = {0};
    cb.on_connected = goWSOnConnected;
    cb.on_message = goWSOnMessage;
    cb.on_closed = goWSOnClosed;
    cb.on_error = goWSOnError;
    return cb;
}
*/
import "C"
import (
	"sync"
	"sync/atomic"
	"unsafe"
)

// Handle registry: maps uint64 IDs to Go objects.
// We pass the ID as void* user_data to C callbacks.

var (
	requestHandles sync.Map // map[uint64]*RequestHandle
	requestSeq     atomic.Uint64

	wsHandles sync.Map // map[uint64]*WSHandle
	wsSeq     atomic.Uint64
)

// RequestHandle holds the Go-side state for an in-flight request.
type RequestHandle struct {
	ID uint64

	// Called when response headers arrive. Return false to cancel.
	OnResponseStarted func(statusCode int, statusText string) bool
	// Called for each data chunk. Return false to cancel.
	OnDataReceived func(data []byte) bool
	// Called when request completes.
	OnCompleted func(netError int)
	// Called on redirect. Return false to cancel.
	OnRedirect func(statusCode int, newURL string) bool
}

// WSHandle holds Go-side state for a WebSocket connection.
type WSHandle struct {
	ID          uint64
	OnConnected func(subprotocol string)
	OnMessage   func(data []byte, isBinary bool)
	OnClosed    func(wasClean bool, code uint16, reason string)
	OnError     func(netError int, message string)
}

// RegisterRequestHandle stores a handle and returns its ID.
func RegisterRequestHandle(h *RequestHandle) uint64 {
	id := requestSeq.Add(1)
	h.ID = id
	requestHandles.Store(id, h)
	return id
}

// UnregisterRequestHandle removes a handle.
func UnregisterRequestHandle(id uint64) {
	requestHandles.Delete(id)
}

// RegisterWSHandle stores a WS handle and returns its ID.
func RegisterWSHandle(h *WSHandle) uint64 {
	id := wsSeq.Add(1)
	h.ID = id
	wsHandles.Store(id, h)
	return id
}

// UnregisterWSHandle removes a WS handle.
func UnregisterWSHandle(id uint64) {
	wsHandles.Delete(id)
}

// IDToPointer converts a handle ID to an unsafe.Pointer for C user_data.
func IDToPointer(id uint64) unsafe.Pointer {
	return unsafe.Pointer(uintptr(id))
}

// MakeGoRequestCallbacks returns a C callbacks struct pointing to Go trampolines.
func MakeGoRequestCallbacks() RequestCallbacks {
	return C.makeGoRequestCallbacks()
}

// MakeGoWSCallbacks returns a C WS callbacks struct pointing to Go trampolines.
func MakeGoWSCallbacks() WebSocketCallbacks {
	return C.makeGoWSCallbacks()
}

// --- Go callback trampolines (called from C) ---

//export goOnResponseStarted
func goOnResponseStarted(req *C.CronetRequest, statusCode C.int, statusText *C.char, userData unsafe.Pointer) C.int {
	id := uint64(uintptr(userData))
	val, ok := requestHandles.Load(id)
	if !ok {
		return 0
	}
	h := val.(*RequestHandle)
	if h.OnResponseStarted != nil {
		st := ""
		if statusText != nil {
			st = C.GoString(statusText)
		}
		if !h.OnResponseStarted(int(statusCode), st) {
			return 1 // cancel
		}
	}
	return 0
}

//export goOnDataReceived
func goOnDataReceived(req *C.CronetRequest, data *C.char, length C.size_t, userData unsafe.Pointer) C.int {
	id := uint64(uintptr(userData))
	val, ok := requestHandles.Load(id)
	if !ok {
		return 0
	}
	h := val.(*RequestHandle)
	if h.OnDataReceived != nil {
		goData := C.GoBytes(unsafe.Pointer(data), C.int(length))
		if !h.OnDataReceived(goData) {
			return 1 // cancel
		}
	}
	return 0
}

//export goOnCompleted
func goOnCompleted(req *C.CronetRequest, netError C.int, userData unsafe.Pointer) {
	id := uint64(uintptr(userData))
	val, ok := requestHandles.Load(id)
	if !ok {
		return
	}
	h := val.(*RequestHandle)
	if h.OnCompleted != nil {
		h.OnCompleted(int(netError))
	}
}

//export goOnRedirect
func goOnRedirect(req *C.CronetRequest, statusCode C.int, newURL *C.char, userData unsafe.Pointer) C.int {
	id := uint64(uintptr(userData))
	val, ok := requestHandles.Load(id)
	if !ok {
		return 0 // follow by default
	}
	h := val.(*RequestHandle)
	if h.OnRedirect != nil {
		u := ""
		if newURL != nil {
			u = C.GoString(newURL)
		}
		if !h.OnRedirect(int(statusCode), u) {
			return 1 // cancel
		}
	}
	return 0
}

//export goWSOnConnected
func goWSOnConnected(ws *C.CronetWebSocket, subprotocol *C.char, userData unsafe.Pointer) {
	id := uint64(uintptr(userData))
	val, ok := wsHandles.Load(id)
	if !ok {
		return
	}
	h := val.(*WSHandle)
	if h.OnConnected != nil {
		sp := ""
		if subprotocol != nil {
			sp = C.GoString(subprotocol)
		}
		h.OnConnected(sp)
	}
}

//export goWSOnMessage
func goWSOnMessage(ws *C.CronetWebSocket, data *C.char, length C.size_t, isBinary C.int, userData unsafe.Pointer) {
	id := uint64(uintptr(userData))
	val, ok := wsHandles.Load(id)
	if !ok {
		return
	}
	h := val.(*WSHandle)
	if h.OnMessage != nil {
		goData := C.GoBytes(unsafe.Pointer(data), C.int(length))
		h.OnMessage(goData, isBinary != 0)
	}
}

//export goWSOnClosed
func goWSOnClosed(ws *C.CronetWebSocket, wasClean C.int, code C.uint16_t, reason *C.char, userData unsafe.Pointer) {
	id := uint64(uintptr(userData))
	val, ok := wsHandles.Load(id)
	if !ok {
		return
	}
	h := val.(*WSHandle)
	if h.OnClosed != nil {
		r := ""
		if reason != nil {
			r = C.GoString(reason)
		}
		h.OnClosed(wasClean != 0, uint16(code), r)
	}
}

//export goWSOnError
func goWSOnError(ws *C.CronetWebSocket, netError C.int, message *C.char, userData unsafe.Pointer) {
	id := uint64(uintptr(userData))
	val, ok := wsHandles.Load(id)
	if !ok {
		return
	}
	h := val.(*WSHandle)
	if h.OnError != nil {
		msg := ""
		if message != nil {
			msg = C.GoString(message)
		}
		h.OnError(int(netError), msg)
	}
}
