package cronet

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"go-cronet/internal/capi"
)

// WebSocket represents a WebSocket connection.
type WebSocket struct {
	ptr       *capi.WebSocket
	engine    *Engine
	handleID  uint64
	messages  chan wsMessage
	connErr   chan error
	done      chan struct{}
	doneOnce  sync.Once // protects close(done)
	mu        sync.Mutex
	destroyed bool
}

type wsMessage struct {
	data     []byte
	isBinary bool
	err      error
}

// DialWebSocket connects to a WebSocket server.
func (e *Engine) DialWebSocket(url string, headers http.Header) (*WebSocket, error) {
	if e.ptr == nil {
		return nil, fmt.Errorf("cronet: engine is closed")
	}

	ws := &WebSocket{
		engine:   e,
		messages: make(chan wsMessage, 64),
		connErr:  make(chan error, 1),
		done:     make(chan struct{}),
	}

	handle := &capi.WSHandle{
		OnConnected: func(subprotocol string) {
			select {
			case ws.connErr <- nil:
			default:
			}
		},
		OnMessage: func(data []byte, isBinary bool) {
			dataCopy := make([]byte, len(data))
			copy(dataCopy, data)
			select {
			case ws.messages <- wsMessage{data: dataCopy, isBinary: isBinary}:
			case <-ws.done:
			}
		},
		OnClosed: func(wasClean bool, code uint16, reason string) {
			ws.doneOnce.Do(func() { close(ws.done) })
		},
		OnError: func(netError int, message string) {
			err := &NetError{Code: netError, Message: message}
			select {
			case ws.connErr <- err:
			default:
			}
			ws.doneOnce.Do(func() { close(ws.done) })
		},
	}

	ws.handleID = capi.RegisterWSHandle(handle)
	callbacks := capi.MakeGoWSCallbacks()
	ws.ptr = capi.WebSocketCreate(e.ptr, &callbacks, capi.IDToPointer(ws.handleID))
	if ws.ptr == nil {
		capi.UnregisterWSHandle(ws.handleID)
		return nil, fmt.Errorf("cronet: failed to create websocket")
	}

	// Set custom headers
	for key, values := range headers {
		for _, v := range values {
			capi.WebSocketAddHeader(ws.ptr, key, v)
		}
	}

	// Connect
	capi.WebSocketConnect(ws.ptr, url, nil)

	// Wait for connection or error
	err := <-ws.connErr
	if err != nil {
		capi.UnregisterWSHandle(ws.handleID)
		capi.WebSocketDestroy(ws.ptr)
		return nil, err
	}

	return ws, nil
}

// ReadMessage reads the next message from the WebSocket.
func (ws *WebSocket) ReadMessage() (data []byte, isBinary bool, err error) {
	select {
	case msg := <-ws.messages:
		return msg.data, msg.isBinary, nil
	case <-ws.done:
		return nil, false, fmt.Errorf("cronet: websocket closed")
	}
}

// WriteMessage sends a message over the WebSocket.
func (ws *WebSocket) WriteMessage(data []byte, binary bool) error {
	ws.mu.Lock()
	destroyed := ws.destroyed
	ws.mu.Unlock()
	if destroyed {
		return fmt.Errorf("cronet: websocket closed")
	}
	select {
	case <-ws.done:
		return fmt.Errorf("cronet: websocket closed")
	default:
	}
	capi.WebSocketSend(ws.ptr, data, binary)
	return nil
}

// Close closes the WebSocket connection gracefully.
// It initiates the close handshake, waits for completion (up to 5s),
// then cleans up all resources.
func (ws *WebSocket) Close(code uint16, reason string) error {
	ws.mu.Lock()
	if ws.destroyed {
		ws.mu.Unlock()
		return nil
	}
	ws.destroyed = true
	ws.mu.Unlock()

	// Initiate WebSocket close handshake
	capi.WebSocketClose(ws.ptr, code, reason)

	// Wait for the close handshake to complete or timeout
	select {
	case <-ws.done:
	case <-time.After(5 * time.Second):
	}

	// Ensure done is closed so ReadMessage/WriteMessage return immediately
	ws.doneOnce.Do(func() { close(ws.done) })

	// Clean up C resources
	capi.UnregisterWSHandle(ws.handleID)
	capi.WebSocketDestroy(ws.ptr)
	return nil
}
