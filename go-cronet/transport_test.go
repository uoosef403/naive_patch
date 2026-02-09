package cronet_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"go-cronet"
)

func TestRoundTripper(t *testing.T) {
	setupTestServers(t)
	cfg := cronet.DefaultEngineConfig()
	cfg.InsecureSkipVerify = true
	engine, err := cronet.NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer engine.Close()

	transport := &cronet.Transport{Engine: engine}
	var _ http.RoundTripper = transport // compile-time interface check

	req, _ := http.NewRequest("GET", httpsURL("/echo"), nil)
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestWithStdHttpClient(t *testing.T) {
	setupTestServers(t)
	cfg := cronet.DefaultEngineConfig()
	cfg.InsecureSkipVerify = true
	engine, err := cronet.NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer engine.Close()

	transport := &cronet.Transport{Engine: engine}
	client := &http.Client{Transport: transport}

	resp, err := client.Get(httpsURL("/echo"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("unmarshal: %v (body: %s)", err, string(body))
	}
	if result["method"] != "GET" {
		t.Fatalf("expected GET, got %s", result["method"])
	}
}

func TestConcurrent100(t *testing.T) {
	setupTestServers(t)
	cfg := cronet.DefaultEngineConfig()
	cfg.InsecureSkipVerify = true
	engine, err := cronet.NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer engine.Close()

	transport := &cronet.Transport{Engine: engine}

	const n = 100
	var wg sync.WaitGroup
	errors := make(chan error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest("GET", httpsURL("/echo"), nil)
			resp, err := transport.RoundTrip(req)
			if err != nil {
				errors <- err
				return
			}
			io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode != 200 {
				errors <- &cronet.NetError{Code: resp.StatusCode, Message: "bad status"}
			}
		}()
	}

	wg.Wait()
	close(errors)
	for err := range errors {
		t.Errorf("concurrent request failed: %v", err)
	}
}

func TestAllHTTPMethods(t *testing.T) {
	setupTestServers(t)
	cfg := cronet.DefaultEngineConfig()
	cfg.InsecureSkipVerify = true
	engine, err := cronet.NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer engine.Close()

	transport := &cronet.Transport{Engine: engine}

	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			var body io.Reader
			if method == "POST" || method == "PUT" || method == "PATCH" {
				body = strings.NewReader("test body")
			}
			req, _ := http.NewRequest(method, httpsURL("/echo"), body)
			if body != nil {
				req.Header.Set("Content-Type", "text/plain")
			}
			resp, err := transport.RoundTrip(req)
			if err != nil {
				t.Fatalf("RoundTrip %s: %v", method, err)
			}
			defer resp.Body.Close()
			io.ReadAll(resp.Body)

			if resp.StatusCode != 200 {
				t.Fatalf("expected 200, got %d", resp.StatusCode)
			}
		})
	}
}

func TestTransportTimeout(t *testing.T) {
	setupTestServers(t)
	cfg := cronet.DefaultEngineConfig()
	cfg.InsecureSkipVerify = true
	engine, err := cronet.NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer engine.Close()

	transport := &cronet.Transport{
		Engine:  engine,
		Timeout: 200 * time.Millisecond,
	}

	req, _ := http.NewRequest("GET", httpsURL("/delay/5000"), nil)
	_, err = transport.RoundTrip(req)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	t.Logf("got expected error: %v", err)
}

func TestTransportNoTimeout(t *testing.T) {
	setupTestServers(t)
	cfg := cronet.DefaultEngineConfig()
	cfg.InsecureSkipVerify = true
	engine, err := cronet.NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer engine.Close()

	transport := &cronet.Transport{Engine: engine}

	req, _ := http.NewRequest("GET", httpsURL("/delay/100"), nil)
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()
	io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}
