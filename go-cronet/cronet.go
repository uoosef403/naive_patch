package cronet

import (
	"net/url"
	"runtime"
	"sync"

	"go-cronet/internal/capi"
)

// EngineConfig controls Engine creation.
type EngineConfig struct {
	UserAgent    string
	EnableQUIC   bool // HTTP/3 support
	EnableHTTP2  bool // default true
	EnableBrotli bool // default true
	EnableZstd   bool
	ProxyURL     string
	CachePath    string
	CacheMaxBytes int

	// TLS configuration
	InsecureSkipVerify bool   // accept any TLS certificate
	CustomCAPEM        string // PEM-encoded root CA bundle
	TLSMinVersion      uint16 // 0 = default; use crypto/tls constants
	TLSMaxVersion      uint16

	// Cookies
	EnableCookies bool
}

// DefaultEngineConfig returns sensible defaults.
func DefaultEngineConfig() EngineConfig {
	return EngineConfig{
		EnableHTTP2:   true,
		EnableBrotli:  true,
		EnableCookies: true,
	}
}

// Engine manages the underlying Chromium network stack.
// It is safe for concurrent use.
type Engine struct {
	ptr    *capi.Engine
	mu     sync.Mutex
	closed bool
	config EngineConfig
}

// NewEngine creates a new Chromium network engine.
func NewEngine(cfg EngineConfig) (*Engine, error) {
	params, cstrs := capi.NewEngineParams(
		cfg.UserAgent,
		cfg.EnableQUIC, cfg.EnableHTTP2, cfg.EnableBrotli, cfg.EnableZstd,
		cfg.ProxyURL, cfg.CachePath, cfg.CacheMaxBytes,
		cfg.InsecureSkipVerify, cfg.CustomCAPEM,
		int(cfg.TLSMinVersion), int(cfg.TLSMaxVersion),
		cfg.EnableCookies,
	)
	defer capi.FreeStrings(cstrs)

	ptr := capi.EngineCreate(&params)
	if ptr == nil {
		return nil, &NetError{Code: -1, Message: "failed to create engine"}
	}

	e := &Engine{ptr: ptr, config: cfg}
	runtime.SetFinalizer(e, (*Engine).Close)
	return e, nil
}

// Close destroys the engine and releases resources.
func (e *Engine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return nil
	}
	e.closed = true
	runtime.SetFinalizer(e, nil)
	capi.EngineDestroy(e.ptr)
	e.ptr = nil
	return nil
}

// SetCookie sets a cookie for the given URL.
// cookieLine is in Set-Cookie format, e.g. "name=value; path=/".
func (e *Engine) SetCookie(u *url.URL, cookieLine string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return
	}
	capi.EngineSetCookie(e.ptr, u.String(), cookieLine)
}

// GetCookies returns all cookies for the given URL as a semicolon-separated string.
func (e *Engine) GetCookies(u *url.URL) string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return ""
	}
	return capi.EngineGetCookies(e.ptr, u.String())
}

// ClearCookies removes all cookies.
func (e *Engine) ClearCookies() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return
	}
	capi.EngineClearCookies(e.ptr)
}

// Version returns the Chromium version string.
func Version() string {
	return capi.Version()
}

// UserAgentString returns the default user agent string.
func UserAgentString() string {
	return capi.UserAgentString()
}
