package cronet_test

import (
	"io"
	"testing"

	"go-cronet"
)

func TestH3WithQUICEnabled(t *testing.T) {
	setupTestServers(t)
	// Note: Our simple test server doesn't support QUIC/H3,
	// but we can verify the engine starts with QUIC enabled
	// and falls back gracefully to H2/H1.
	cfg := cronet.EngineConfig{
		EnableQUIC:         true,
		EnableHTTP2:        true,
		InsecureSkipVerify: true,
	}
	engine, err := cronet.NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine with QUIC: %v", err)
	}
	defer engine.Close()

	transport := &cronet.Transport{Engine: engine}
	req, _ := newRequest("GET", httpsURL("/echo"), nil)
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()
	io.ReadAll(resp.Body)

	t.Logf("Proto: %s (QUIC enabled, server may not support H3)", resp.Proto)
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestQUICDisabledFallback(t *testing.T) {
	setupTestServers(t)
	// With QUIC disabled, should use H2 or H1.1
	cfg := cronet.EngineConfig{
		EnableQUIC:         false,
		EnableHTTP2:        true,
		InsecureSkipVerify: true,
	}
	engine, err := cronet.NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer engine.Close()

	transport := &cronet.Transport{Engine: engine}
	req, _ := newRequest("GET", httpsURL("/echo"), nil)
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()
	io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	// Should NOT be HTTP/3
	if resp.Proto == "HTTP/3" {
		t.Fatal("expected non-H3 protocol when QUIC is disabled")
	}
	t.Logf("Proto: %s (QUIC disabled)", resp.Proto)
}

func TestH3FallbackToH2(t *testing.T) {
	setupTestServers(t)
	// Enable QUIC but connect to server without H3 -> should fallback
	cfg := cronet.EngineConfig{
		EnableQUIC:         true,
		EnableHTTP2:        true,
		InsecureSkipVerify: true,
	}
	engine, err := cronet.NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer engine.Close()

	transport := &cronet.Transport{Engine: engine}

	// Multiple requests to verify stable fallback
	for i := 0; i < 3; i++ {
		req, _ := newRequest("GET", httpsURL("/echo"), nil)
		resp, err := transport.RoundTrip(req)
		if err != nil {
			t.Fatalf("RoundTrip %d: %v", i, err)
		}
		io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != 200 {
			t.Fatalf("request %d: expected 200, got %d", i, resp.StatusCode)
		}
	}
}
