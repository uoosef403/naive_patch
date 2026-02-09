package cronet

import (
	"fmt"
	"net/http"
	"sync"

	"go-cronet/internal/capi"
)

// WebSocket represents a WebSocket connection.
type WebSocket struct {
	ptr      *capi.WebSocket
	engine   *Engine
	handleID uint64
	messages chan wsMessage
	connErr  chan error
	done     chan struct{}
	closeOnce sync.Once
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
			ws.closeOnce.Do(func() { close(ws.done) })
		},
		OnError: func(netError int, message string) {
			err := &NetError{Code: netError, Message: message}
			select {
			case ws.connErr <- err:
			default:
			}
			ws.closeOnce.Do(func() { close(ws.done) })
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
	select {
	case <-ws.done:
		return fmt.Errorf("cronet: websocket closed")
	default:
	}
	capi.WebSocketSend(ws.ptr, data, binary)
	return nil
}

// Close closes the WebSocket connection.
func (ws *WebSocket) Close(code uint16, reason string) error {
	capi.WebSocketClose(ws.ptr, code, reason)
	// Wait briefly for close to complete
	select {
	case <-ws.done:
	default:
	}
	capi.UnregisterWSHandle(ws.handleID)
	capi.WebSocketDestroy(ws.ptr)
	return nil
}
