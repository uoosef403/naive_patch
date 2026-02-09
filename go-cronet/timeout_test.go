package cronet_test

import (
	"testing"
	"time"

	"go-cronet"
)

func TestRequestTimeout(t *testing.T) {
	setupTestServers(t)
	client, err := cronet.NewClient(
		cronet.WithInsecureSkipVerify(),
		cronet.WithTimeout(200*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	start := time.Now()
	_, err = client.Get(httpsURL("/delay/5000"))
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error")
	}
	t.Logf("timed out after %v: %v", elapsed, err)

	// Should timeout around 200ms, not wait 5000ms
	if elapsed > 2*time.Second {
		t.Fatalf("timeout took too long: %v", elapsed)
	}
}

func TestConnectTimeout(t *testing.T) {
	// 192.0.2.1 is TEST-NET-1 (RFC 5737) - guaranteed non-routable
	client, err := cronet.NewClient(
		cronet.WithConnectTimeout(500*time.Millisecond),
		cronet.WithTimeout(2*time.Second),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	start := time.Now()
	_, err = client.Get("http://192.0.2.1:12345/")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected connection error")
	}
	t.Logf("connection failed after %v: %v", elapsed, err)

	// Should fail relatively quickly
	if elapsed > 10*time.Second {
		t.Fatalf("connect timeout took too long: %v", elapsed)
	}
}

func TestNoTimeoutSlowResponse(t *testing.T) {
	setupTestServers(t)
	client, err := cronet.NewClient(
		cronet.WithInsecureSkipVerify(),
		// No timeout set
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	resp, err := client.Get(httpsURL("/delay/300"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}
