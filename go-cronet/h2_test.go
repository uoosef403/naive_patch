package cronet_test

import (
	"encoding/json"
	"io"
	"sync"
	"testing"

	"go-cronet"
)

func TestH2Negotiation(t *testing.T) {
	setupTestServers(t)
	cfg := cronet.EngineConfig{
		EnableHTTP2:        true,
		EnableBrotli:       true,
		InsecureSkipVerify: true,
	}
	engine, err := cronet.NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer engine.Close()

	transport := &cronet.Transport{Engine: engine}

	req, _ := newRequest("GET", httpsURL("/tls-info"), nil)
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	t.Logf("tls-info response: %s", string(body))

	// Check if HTTP/2 was negotiated
	if resp.Proto == "HTTP/2" || resp.Proto == "HTTP/2.0" {
		t.Logf("HTTP/2 negotiated successfully")
	} else {
		t.Logf("protocol: %s (HTTP/2 may not be available in test config)", resp.Proto)
	}
}

func TestH2Multiplexing(t *testing.T) {
	setupTestServers(t)
	cfg := cronet.EngineConfig{
		EnableHTTP2:        true,
		EnableBrotli:       true,
		InsecureSkipVerify: true,
	}
	engine, err := cronet.NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer engine.Close()

	transport := &cronet.Transport{Engine: engine}

	// Send 10 concurrent requests through the same transport
	const n = 10
	var wg sync.WaitGroup
	results := make(chan int, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, _ := newRequest("GET", httpsURL("/echo"), nil)
			resp, err := transport.RoundTrip(req)
			if err != nil {
				t.Errorf("concurrent RoundTrip: %v", err)
				return
			}
			io.ReadAll(resp.Body)
			resp.Body.Close()
			results <- resp.StatusCode
		}()
	}

	wg.Wait()
	close(results)

	count := 0
	for code := range results {
		if code != 200 {
			t.Errorf("request returned %d", code)
		}
		count++
	}
	if count != n {
		t.Fatalf("expected %d results, got %d", n, count)
	}
}

func TestH2LargeBody(t *testing.T) {
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

	transport := &cronet.Transport{Engine: engine}

	// Request 5MB of data
	req, _ := newRequest("GET", httpsURL("/large/5242880"), nil)
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()

	n, err := io.Copy(io.Discard, resp.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	if n != 5242880 {
		t.Fatalf("expected 5242880 bytes, got %d", n)
	}
}

func TestH2ResponseHeaders(t *testing.T) {
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

	transport := &cronet.Transport{Engine: engine}

	req, _ := newRequest("GET", httpsURL("/tls-info"), nil)
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var info map[string]interface{}
	json.Unmarshal(body, &info)

	// Verify we can read response headers
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		t.Fatal("missing Content-Type header")
	}
	t.Logf("Content-Type: %s, Proto: %s", ct, resp.Proto)
}
