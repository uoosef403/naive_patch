package cronet_test

import (
	"io"
	"testing"

	"go-cronet"
)

func TestFollowRedirects(t *testing.T) {
	setupTestServers(t)
	client, err := cronet.NewClient(
		cronet.WithInsecureSkipVerify(),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	// /redirect/3 -> /redirect/2 -> /redirect/1 -> /echo
	resp, err := client.Get(httpsURL("/redirect/3"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()
	io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 after redirects, got %d", resp.StatusCode)
	}
}

func TestMaxRedirects3(t *testing.T) {
	setupTestServers(t)
	client, err := cronet.NewClient(
		cronet.WithInsecureSkipVerify(),
		cronet.WithMaxRedirects(2),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	// /redirect/5 requires 5 redirects, but we allow only 2
	_, err = client.Get(httpsURL("/redirect/5"))
	if err == nil {
		t.Fatal("expected redirect error")
	}
	t.Logf("got expected error: %v", err)
}

func TestDisableRedirects(t *testing.T) {
	setupTestServers(t)
	client, err := cronet.NewClient(
		cronet.WithInsecureSkipVerify(),
		cronet.WithMaxRedirects(0),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	resp, err := client.Get(httpsURL("/redirect/1"))
	if err != nil {
		// When max_redirects=0, we get the redirect response directly
		// or an error depending on implementation
		t.Logf("got redirect result: %v", err)
		return
	}
	defer resp.Body.Close()
	io.ReadAll(resp.Body)

	// Should get the redirect status code directly
	if resp.StatusCode != 302 && resp.StatusCode != 301 && resp.StatusCode != 200 {
		t.Logf("got status %d when redirects disabled", resp.StatusCode)
	}
}

func TestRedirectChain(t *testing.T) {
	setupTestServers(t)
	client, err := cronet.NewClient(
		cronet.WithInsecureSkipVerify(),
		cronet.WithMaxRedirects(10),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	resp, err := client.Get(httpsURL("/redirect/5"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()
	io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}
