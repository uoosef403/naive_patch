package cronet

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"go-cronet/internal/capi"
)

// Transport implements http.RoundTripper using libcronet.
type Transport struct {
	Engine          *Engine
	Timeout         time.Duration
	ConnectTimeout  time.Duration
	MaxRedirects    int // -1 = default, 0 = no follow
	DisableCache    bool
	RequestPriority Priority
}

var _ http.RoundTripper = (*Transport)(nil)

// RoundTrip executes an HTTP request and returns a response.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.Engine == nil || t.Engine.ptr == nil {
		return nil, fmt.Errorf("cronet: engine is nil or closed")
	}

	urlStr := req.URL.String()
	method := req.Method
	if method == "" {
		method = "GET"
	}

	// Read body if present
	var bodyBytes []byte
	var contentType string
	if req.Body != nil {
		var err error
		bodyBytes, err = io.ReadAll(req.Body)
		req.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("cronet: reading request body: %w", err)
		}
		contentType = req.Header.Get("Content-Type")
	}

	// Set up the response pipe for streaming body
	bodyPR, bodyPW := io.Pipe()

	// Response info collected from callbacks
	type respInfo struct {
		statusCode  int
		statusText  string
	}
	var info respInfo

	headersReady := make(chan struct{})
	var headersOnce sync.Once
	requestError := make(chan error, 1)

	// Create callback handle
	handle := &capi.RequestHandle{
		OnResponseStarted: func(statusCode int, statusText string) bool {
			info.statusCode = statusCode
			info.statusText = statusText
			headersOnce.Do(func() { close(headersReady) })
			return true
		},
		OnDataReceived: func(data []byte) bool {
			_, err := bodyPW.Write(data)
			return err == nil
		},
		OnCompleted: func(netError int) {
			if netError != 0 {
				err := NewNetError(netError)
				bodyPW.CloseWithError(err)
				select {
				case requestError <- err:
				default:
				}
			} else {
				bodyPW.Close()
			}
			headersOnce.Do(func() { close(headersReady) })
		},
		OnRedirect: func(statusCode int, newURL string) bool {
			return true // follow all redirects (Transport delegates to cronet)
		},
	}

	handleID := capi.RegisterRequestHandle(handle)

	// Create the C request
	callbacks := capi.MakeGoRequestCallbacks()
	creq := capi.RequestCreate(t.Engine.ptr, method, urlStr, &callbacks, capi.IDToPointer(handleID))
	if creq == nil {
		capi.UnregisterRequestHandle(handleID)
		bodyPR.Close()
		bodyPW.Close()
		return nil, fmt.Errorf("cronet: failed to create request")
	}

	// Set headers (skip Host and Content-Type which are handled separately)
	for key, values := range req.Header {
		if strings.EqualFold(key, "Content-Type") && contentType != "" {
			continue
		}
		for _, v := range values {
			capi.RequestAddHeader(creq, key, v)
		}
	}

	// Set body
	if len(bodyBytes) > 0 {
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		capi.RequestSetBody(creq, contentType, bodyBytes)
	}

	// Set params
	params := capi.NewRequestParams(
		int(t.Timeout.Milliseconds()),
		int(t.ConnectTimeout.Milliseconds()),
		t.MaxRedirects,
		int(t.RequestPriority),
		t.DisableCache,
	)
	capi.RequestSetParams(creq, &params)

	// Start the request
	capi.RequestStart(creq)

	// Wait for headers or error
	<-headersReady

	if info.statusCode == 0 {
		// Request failed before headers arrived
		bodyPR.Close()
		capi.UnregisterRequestHandle(handleID)
		capi.RequestDestroy(creq)
		select {
		case err := <-requestError:
			return nil, err
		default:
			return nil, fmt.Errorf("cronet: request failed before headers")
		}
	}

	// Collect response info from C request
	headers := make(http.Header)
	headerCount := capi.RequestGetHeaderCount(creq)
	for i := 0; i < headerCount; i++ {
		name := capi.RequestGetHeaderName(creq, i)
		value := capi.RequestGetHeaderValue(creq, i)
		headers.Add(name, value)
	}

	httpVersion := capi.RequestGetHTTPVersion(creq)
	contentLen := capi.RequestGetContentLength(creq)
	protoMajor, protoMinor := parseHTTPVersion(httpVersion)

	resp := &http.Response{
		Status:        fmt.Sprintf("%d %s", info.statusCode, info.statusText),
		StatusCode:    info.statusCode,
		Proto:         httpVersion,
		ProtoMajor:    protoMajor,
		ProtoMinor:    protoMinor,
		Header:        headers,
		Body:          &requestBody{reader: bodyPR, req: creq, handleID: handleID},
		ContentLength: contentLen,
		Request:       req,
	}

	return resp, nil
}

// requestBody wraps the pipe reader and ensures cleanup on Close.
type requestBody struct {
	reader   *io.PipeReader
	req      *capi.Request
	handleID uint64
	closed   bool
}

func (rb *requestBody) Read(p []byte) (int, error) {
	return rb.reader.Read(p)
}

func (rb *requestBody) Close() error {
	if rb.closed {
		return nil
	}
	rb.closed = true
	err := rb.reader.Close()
	capi.UnregisterRequestHandle(rb.handleID)
	if rb.req != nil {
		capi.RequestDestroy(rb.req)
		rb.req = nil
	}
	return err
}

func parseHTTPVersion(s string) (major, minor int) {
	switch s {
	case "HTTP/3":
		return 3, 0
	case "HTTP/2":
		return 2, 0
	case "HTTP/1.1":
		return 1, 1
	case "HTTP/1.0":
		return 1, 0
	default:
		return 1, 1
	}
}
