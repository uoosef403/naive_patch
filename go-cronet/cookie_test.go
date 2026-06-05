package cronet_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"testing"

	"go-cronet"
)

func TestSetGetCookie(t *testing.T) {
	setupTestServers(t)
	cfg := cronet.EngineConfig{
		EnableHTTP2:        true,
		EnableBrotli:       true,
		EnableCookies:      true,
		InsecureSkipVerify: true,
	}
	engine, err := cronet.NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer engine.Close()

	u, _ := url.Parse(httpsURL("/"))
	engine.SetCookie(u, "testkey=testvalue; path=/")

	raw := engine.GetCookies(u)
	if raw == "" {
		t.Fatal("GetCookies returned empty")
	}
	t.Logf("cookies: %s", raw)

	if raw != "testkey=testvalue" {
		t.Fatalf("expected testkey=testvalue, got %q", raw)
	}
}

func TestClearCookies(t *testing.T) {
	setupTestServers(t)
	cfg := cronet.EngineConfig{
		EnableHTTP2:        true,
		EnableBrotli:       true,
		EnableCookies:      true,
		InsecureSkipVerify: true,
	}
	engine, err := cronet.NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer engine.Close()

	u, _ := url.Parse(httpsURL("/"))
	engine.SetCookie(u, "clearme=please; path=/")

	raw := engine.GetCookies(u)
	if raw == "" {
		t.Fatal("cookie not set")
	}

	engine.ClearCookies()

	raw = engine.GetCookies(u)
	if raw != "" {
		t.Fatalf("expected empty after clear, got %q", raw)
	}
}

func TestServerSetCookie(t *testing.T) {
	setupTestServers(t)
	cfg := cronet.EngineConfig{
		EnableHTTP2:        true,
		EnableBrotli:       true,
		EnableCookies:      true,
		InsecureSkipVerify: true,
	}
	engine, err := cronet.NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer engine.Close()

	transport := &cronet.Transport{Engine: engine}

	// First request sets cookies via server
	req, _ := http.NewRequest("GET", httpsURL("/cookies/set?flavor=chocolate&size=large"), nil)
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip set: %v", err)
	}
	resp.Body.Close()

	// Second request should send cookies back
	req2, _ := http.NewRequest("GET", httpsURL("/cookies/get"), nil)
	resp2, err := transport.RoundTrip(req2)
	if err != nil {
		t.Fatalf("RoundTrip get: %v", err)
	}
	defer resp2.Body.Close()

	body, _ := io.ReadAll(resp2.Body)
	var cookies map[string]string
	json.Unmarshal(body, &cookies)

	if cookies["flavor"] != "chocolate" {
		t.Fatalf("expected flavor=chocolate, got %v", cookies)
	}
	if cookies["size"] != "large" {
		t.Fatalf("expected size=large, got %v", cookies)
	}
}

func TestServerSetCookieHTTP(t *testing.T) {
	setupTestServers(t)
	cfg := cronet.EngineConfig{
		EnableHTTP2:   true,
		EnableBrotli:  true,
		EnableCookies: true,
	}
	engine, err := cronet.NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer engine.Close()

	transport := &cronet.Transport{Engine: engine}

	// First request sets cookies via server over plain HTTP
	req, _ := http.NewRequest("GET", httpURL("/cookies/set?color=blue&shape=circle"), nil)
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip set: %v", err)
	}
	resp.Body.Close()

	// Second request should send cookies back
	req2, _ := http.NewRequest("GET", httpURL("/cookies/get"), nil)
	resp2, err := transport.RoundTrip(req2)
	if err != nil {
		t.Fatalf("RoundTrip get: %v", err)
	}
	defer resp2.Body.Close()

	body, _ := io.ReadAll(resp2.Body)
	var cookies map[string]string
	json.Unmarshal(body, &cookies)

	if cookies["color"] != "blue" {
		t.Fatalf("expected color=blue, got %v", cookies)
	}
	if cookies["shape"] != "circle" {
		t.Fatalf("expected shape=circle, got %v", cookies)
	}
}

func TestCookieJarInterface(t *testing.T) {
	setupTestServers(t)
	cfg := cronet.EngineConfig{
		EnableHTTP2:        true,
		EnableBrotli:       true,
		EnableCookies:      true,
		InsecureSkipVerify: true,
	}
	engine, err := cronet.NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer engine.Close()

	jar := cronet.NewCookieJar(engine)
	var _ http.CookieJar = jar // compile-time interface check

	u, _ := url.Parse(httpsURL("/"))
	jar.SetCookies(u, []*http.Cookie{
		{Name: "go_cookie", Value: "go_value", Path: "/"},
	})

	cookies := jar.Cookies(u)
	if len(cookies) == 0 {
		t.Fatal("no cookies returned from jar")
	}

	found := false
	for _, c := range cookies {
		if c.Name == "go_cookie" && c.Value == "go_value" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("cookie not found, got: %+v", cookies)
	}
}
