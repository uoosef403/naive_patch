package cronet_test

import (
	"testing"

	"go-cronet"
)

func TestWSAPICompiles(t *testing.T) {
	// Verify the WebSocket API types exist and compile
	cfg := cronet.EngineConfig{
		EnableHTTP2:        true,
		InsecureSkipVerify: true,
	}
	engine, err := cronet.NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer engine.Close()

	// Test that DialWebSocket API exists
	// We don't actually connect since we'd need a WS server
	_, err = engine.DialWebSocket("wss://127.0.0.1:1/ws", nil)
	if err == nil {
		t.Fatal("expected error connecting to invalid WS endpoint")
	}
	t.Logf("expected WS connection error: %v", err)
}
