package cronet

import (
	"io"
	"net/http"
	"strings"
	"time"
)

// Option configures a Client.
type Option func(*clientConfig)

type clientConfig struct {
	engineCfg   EngineConfig
	timeout     time.Duration
	connTimeout time.Duration
	maxRedir    int
	priority    Priority
	noCache     bool
}

// WithUserAgent sets the User-Agent header.
func WithUserAgent(ua string) Option {
	return func(c *clientConfig) { c.engineCfg.UserAgent = ua }
}

// WithQUIC enables or disables HTTP/3 (QUIC) support.
func WithQUIC(enable bool) Option {
	return func(c *clientConfig) { c.engineCfg.EnableQUIC = enable }
}

// WithHTTP2 enables or disables HTTP/2 support.
func WithHTTP2(enable bool) Option {
	return func(c *clientConfig) { c.engineCfg.EnableHTTP2 = enable }
}

// WithBrotli enables or disables Brotli decompression.
func WithBrotli(enable bool) Option {
	return func(c *clientConfig) { c.engineCfg.EnableBrotli = enable }
}

// WithZstd enables or disables Zstd decompression.
func WithZstd(enable bool) Option {
	return func(c *clientConfig) { c.engineCfg.EnableZstd = enable }
}

// WithProxy sets the proxy URL.
func WithProxy(url string) Option {
	return func(c *clientConfig) { c.engineCfg.ProxyURL = url }
}

// WithCachePath enables disk cache at the given path.
func WithCachePath(path string, maxBytes int) Option {
	return func(c *clientConfig) {
		c.engineCfg.CachePath = path
		c.engineCfg.CacheMaxBytes = maxBytes
	}
}

// WithInsecureSkipVerify disables TLS certificate verification.
func WithInsecureSkipVerify() Option {
	return func(c *clientConfig) { c.engineCfg.InsecureSkipVerify = true }
}

// WithCustomCA sets a PEM-encoded CA certificate bundle.
func WithCustomCA(pem string) Option {
	return func(c *clientConfig) { c.engineCfg.CustomCAPEM = pem }
}

// WithTLSVersion sets the minimum and maximum TLS version.
// Use crypto/tls constants: tls.VersionTLS12, tls.VersionTLS13, etc.
func WithTLSVersion(min, max uint16) Option {
	return func(c *clientConfig) {
		c.engineCfg.TLSMinVersion = min
		c.engineCfg.TLSMaxVersion = max
	}
}

// WithCookies enables or disables the cookie jar.
func WithCookies(enable bool) Option {
	return func(c *clientConfig) { c.engineCfg.EnableCookies = enable }
}

// WithTimeout sets the overall request timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *clientConfig) { c.timeout = d }
}

// WithConnectTimeout sets the connection timeout.
func WithConnectTimeout(d time.Duration) Option {
	return func(c *clientConfig) { c.connTimeout = d }
}

// WithMaxRedirects sets the maximum number of redirects to follow.
// 0 means no redirects, -1 means default.
func WithMaxRedirects(n int) Option {
	return func(c *clientConfig) { c.maxRedir = n }
}

// WithPriority sets the default request priority.
func WithPriority(p Priority) Option {
	return func(c *clientConfig) { c.priority = p }
}

// WithNoCache disables caching for all requests.
func WithNoCache() Option {
	return func(c *clientConfig) { c.noCache = true }
}

// Client is a high-level HTTP client powered by Chromium's network stack.
type Client struct {
	Engine    *Engine
	Transport *Transport
}

// NewClient creates a Client with the given options.
func NewClient(opts ...Option) (*Client, error) {
	cfg := clientConfig{
		engineCfg: DefaultEngineConfig(),
		maxRedir:  -1,
	}
	for _, o := range opts {
		o(&cfg)
	}

	engine, err := NewEngine(cfg.engineCfg)
	if err != nil {
		return nil, err
	}

	t := &Transport{
		Engine:          engine,
		Timeout:         cfg.timeout,
		ConnectTimeout:  cfg.connTimeout,
		MaxRedirects:    cfg.maxRedir,
		DisableCache:    cfg.noCache,
		RequestPriority: cfg.priority,
	}

	return &Client{Engine: engine, Transport: t}, nil
}

// Do executes an HTTP request.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	return c.Transport.RoundTrip(req)
}

// Get performs an HTTP GET request.
func (c *Client) Get(url string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

// Post performs an HTTP POST request.
func (c *Client) Post(url, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return c.Do(req)
}

// Head performs an HTTP HEAD request.
func (c *Client) Head(url string) (*http.Response, error) {
	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

// PostForm performs an HTTP POST with form data.
func (c *Client) PostForm(url string, data map[string]string) (*http.Response, error) {
	var pairs []string
	for k, v := range data {
		pairs = append(pairs, k+"="+v)
	}
	body := strings.NewReader(strings.Join(pairs, "&"))
	return c.Post(url, "application/x-www-form-urlencoded", body)
}

// Close shuts down the client and releases all resources.
func (c *Client) Close() error {
	if c.Engine != nil {
		return c.Engine.Close()
	}
	return nil
}

// StdClient returns an *http.Client using this Client's Transport.
// Useful for drop-in replacement of Go's standard HTTP client.
func (c *Client) StdClient() *http.Client {
	return &http.Client{Transport: c.Transport}
}
