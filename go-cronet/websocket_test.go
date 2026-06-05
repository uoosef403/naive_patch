package cronet_test

import (
	"bytes"
	"testing"
	"time"

	"go-cronet"
)

func TestWebSocketEcho(t *testing.T) {
	setupTestServers(t)
	cfg := cronet.EngineConfig{
		EnableHTTP2:        true,
		InsecureSkipVerify: true,
	}
	engine, err := cronet.NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer engine.Close()

	ws, err := engine.DialWebSocket(wsURL("/ws/echo"), nil)
	if err != nil {
		t.Fatalf("DialWebSocket: %v", err)
	}

	// Small delay to ensure the WebSocket channel is fully initialized
	time.Sleep(100 * time.Millisecond)

	// Send text message
	msg := []byte("hello cronet websocket")
	t.Logf("Sending message: %q", msg)
	if err := ws.WriteMessage(msg, false); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}
	t.Log("WriteMessage returned")

	// Read echo with timeout
	done := make(chan struct{})
	var data []byte
	var isBinary bool
	go func() {
		data, isBinary, err = ws.ReadMessage()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("ReadMessage timed out after 10s")
	}
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if isBinary {
		t.Fatal("expected text message, got binary")
	}
	if string(data) != string(msg) {
		t.Fatalf("expected %q, got %q", msg, data)
	}

	ws.Close(1000, "done")
}

func TestWebSocketBinary(t *testing.T) {
	setupTestServers(t)
	cfg := cronet.EngineConfig{
		EnableHTTP2:        true,
		InsecureSkipVerify: true,
	}
	engine, err := cronet.NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer engine.Close()

	ws, err := engine.DialWebSocket(wsURL("/ws/echo"), nil)
	if err != nil {
		t.Fatalf("DialWebSocket: %v", err)
	}

	// Send binary data
	binData := []byte{0x00, 0x01, 0x02, 0xFF, 0xFE, 0xFD}
	if err := ws.WriteMessage(binData, true); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}

	data, isBinary, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if !isBinary {
		t.Fatal("expected binary message, got text")
	}
	if !bytes.Equal(data, binData) {
		t.Fatalf("binary data mismatch: got %x, want %x", data, binData)
	}

	ws.Close(1000, "done")
}

func TestWebSocketWSSEcho(t *testing.T) {
	setupTestServers(t)
	cfg := cronet.EngineConfig{
		EnableHTTP2:        true,
		InsecureSkipVerify: true,
	}
	engine, err := cronet.NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer engine.Close()

	ws, err := engine.DialWebSocket(wssURL("/ws/echo"), nil)
	if err != nil {
		t.Fatalf("DialWebSocket wss: %v", err)
	}

	msg := []byte("hello wss")
	if err := ws.WriteMessage(msg, false); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}

	data, isBinary, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if isBinary {
		t.Fatal("expected text message, got binary")
	}
	if string(data) != string(msg) {
		t.Fatalf("expected %q, got %q", msg, data)
	}

	ws.Close(1000, "done")
}

func TestWebSocketClose(t *testing.T) {
	setupTestServers(t)
	cfg := cronet.EngineConfig{
		EnableHTTP2:        true,
		InsecureSkipVerify: true,
	}
	engine, err := cronet.NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer engine.Close()

	ws, err := engine.DialWebSocket(wsURL("/ws/echo"), nil)
	if err != nil {
		t.Fatalf("DialWebSocket: %v", err)
	}

	// Initiate clean close
	ws.Close(1000, "normal closure")

	// After close, reading should fail
	done := make(chan struct{})
	go func() {
		_, _, err := ws.ReadMessage()
		if err == nil {
			t.Error("expected error after close, got nil")
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("ReadMessage did not return after close within timeout")
	}
}

func TestWebSocketConnectError(t *testing.T) {
	cfg := cronet.EngineConfig{
		EnableHTTP2:        true,
		InsecureSkipVerify: true,
	}
	engine, err := cronet.NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer engine.Close()

	// Connect to invalid endpoint
	_, err = engine.DialWebSocket("wss://127.0.0.1:1/ws", nil)
	if err == nil {
		t.Fatal("expected error connecting to invalid WS endpoint")
	}
	t.Logf("expected WS connection error: %v", err)
}
