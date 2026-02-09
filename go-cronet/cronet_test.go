package cronet_test

import (
	"testing"

	"go-cronet"
)

func TestVersion(t *testing.T) {
	v := cronet.Version()
	if v == "" {
		t.Fatal("Version() returned empty string")
	}
	t.Logf("Chromium version: %s", v)
}

func TestUserAgentString(t *testing.T) {
	ua := cronet.UserAgentString()
	if ua == "" {
		t.Fatal("UserAgentString() returned empty string")
	}
	t.Logf("User-Agent: %s", ua)
}

func TestEngineCreateDestroy(t *testing.T) {
	cfg := cronet.DefaultEngineConfig()
	engine, err := cronet.NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	if err := engine.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestEngineDoubleClose(t *testing.T) {
	cfg := cronet.DefaultEngineConfig()
	engine, err := cronet.NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	if err := engine.Close(); err != nil {
		t.Fatalf("Close 1: %v", err)
	}
	// Second close should be a no-op.
	if err := engine.Close(); err != nil {
		t.Fatalf("Close 2: %v", err)
	}
}

func TestMultipleEngines(t *testing.T) {
	engines := make([]*cronet.Engine, 3)
	for i := range engines {
		cfg := cronet.DefaultEngineConfig()
		e, err := cronet.NewEngine(cfg)
		if err != nil {
			t.Fatalf("NewEngine %d: %v", i, err)
		}
		engines[i] = e
	}
	for i, e := range engines {
		if err := e.Close(); err != nil {
			t.Fatalf("Close %d: %v", i, err)
		}
	}
}

func TestEngineWithAllOptions(t *testing.T) {
	cfg := cronet.EngineConfig{
		UserAgent:          "TestAgent/1.0",
		EnableQUIC:         true,
		EnableHTTP2:        true,
		EnableBrotli:       true,
		EnableZstd:         true,
		InsecureSkipVerify: true,
		EnableCookies:      true,
	}
	engine, err := cronet.NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer engine.Close()
}

func TestNetError(t *testing.T) {
	err := cronet.NewNetError(-7)
	if err == nil {
		t.Fatal("expected error for code -7")
	}
	ne, ok := err.(*cronet.NetError)
	if !ok {
		t.Fatalf("expected *NetError, got %T", err)
	}
	if ne.Code != -7 {
		t.Fatalf("expected code -7, got %d", ne.Code)
	}
	if ne.Message == "" {
		t.Fatal("expected non-empty error message")
	}
	t.Logf("NetError(-7): %s", err)

	// Code 0 should return nil
	if e := cronet.NewNetError(0); e != nil {
		t.Fatalf("expected nil for code 0, got %v", e)
	}
}
