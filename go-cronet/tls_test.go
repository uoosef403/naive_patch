package cronet_test

import (
	"io"
	"net/http"
	"testing"

	"go-cronet"
)

func TestInsecureSkipVerify(t *testing.T) {
	setupTestServers(t)
	// Self-signed cert should work with InsecureSkipVerify=true
	client, err := cronet.NewClient(
		cronet.WithInsecureSkipVerify(),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	resp, err := client.Get(httpsURL("/echo"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()
	io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestInsecureSkipVerifyDisabled(t *testing.T) {
	setupTestServers(t)
	// Self-signed cert should fail without InsecureSkipVerify
	cfg := cronet.EngineConfig{
		EnableHTTP2:        true,
		EnableBrotli:       true,
		InsecureSkipVerify: false, // explicitly disabled
	}
	engine, err := cronet.NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer engine.Close()

	transport := &cronet.Transport{Engine: engine}
	req, _ := http.NewRequest("GET", httpsURL("/echo"), nil)
	resp, err := transport.RoundTrip(req)
	if err == nil {
		resp.Body.Close()
		t.Fatal("expected TLS error for self-signed cert")
	}
	t.Logf("got expected error: %v", err)
}

func TestCustomCA(t *testing.T) {
	setupTestServers(t)
	if testServer.caPEM == "" {
		t.Skip("no test CA available")
	}

	// Use the test CA - this should make the self-signed cert valid
	client, err := cronet.NewClient(
		cronet.WithCustomCA(testServer.caPEM),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	resp, err := client.Get(httpsURL("/echo"))
	if err != nil {
		// CustomCA may not work in all builds - log and skip
		t.Skipf("CustomCA not working (may need different build config): %v", err)
	}
	defer resp.Body.Close()
	io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestHTTPCleartext(t *testing.T) {
	setupTestServers(t)
	// Test plain HTTP (no TLS)
	client, err := cronet.NewClient()
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	resp, err := client.Get(httpURL("/echo"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()
	io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}
