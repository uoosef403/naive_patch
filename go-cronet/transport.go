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

	// Buffered channel for streaming body data from the IO thread.
	// Using a channel avoids blocking the Chromium IO thread (unlike io.Pipe).
	dataCh := make(chan []byte, 32)
	// Closed when OnCompleted fires; signals the request is fully done.
	completeCh := make(chan struct{})
	// Closed by requestBody.Close() to unblock a blocked data callback.
	cancelCh := make(chan struct{})

	headersReady := make(chan struct{})
	var headersOnce sync.Once

	// Response info collected from callbacks.
	// Written on the IO thread before signaling headersReady/completeCh,
	// read on the caller goroutine after receiving from those channels.
	var info struct {
		statusCode    int
		statusText    string
		completionErr error // set before close(dataCh)
	}

	// Create callback handle
	handle := &capi.RequestHandle{
		OnResponseStarted: func(statusCode int, statusText string) bool {
			info.statusCode = statusCode
			info.statusText = statusText
			headersOnce.Do(func() { close(headersReady) })
			return true
		},
		OnDataReceived: func(data []byte) bool {
			// Copy data — the C buffer is only valid for this call.
			dataCopy := make([]byte, len(data))
			copy(dataCopy, data)
			select {
			case dataCh <- dataCopy:
				return true
			case <-cancelCh:
				return false // body was closed; cancel the request
			}
		},
		OnCompleted: func(netError int) {
			if netError != 0 {
				info.completionErr = NewNetError(netError)
			}
			close(dataCh)
			close(completeCh)
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
		return nil, fmt.Errorf("cronet: failed to create request")
	}

	// Set headers (skip Content-Type which is handled with the body)
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
		// Request failed before headers arrived.
		// Wait for the IO thread to fully complete before destroying.
		<-completeCh
		capi.UnregisterRequestHandle(handleID)
		capi.RequestDestroy(creq)
		if info.completionErr != nil {
			return nil, info.completionErr
		}
		return nil, fmt.Errorf("cronet: request failed before headers")
	}

	// Collect response info from C request (safe: headers are cached and
	// immutable after OnResponseStarted returns).
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
		Status:     fmt.Sprintf("%d %s", info.statusCode, info.statusText),
		StatusCode: info.statusCode,
		Proto:      httpVersion,
		ProtoMajor: protoMajor,
		ProtoMinor: protoMinor,
		Header:     headers,
		Body: &requestBody{
			dataCh:        dataCh,
			completeCh:    completeCh,
			cancelCh:      cancelCh,
			completionErr: &info.completionErr,
			req:           creq,
			handleID:      handleID,
		},
		ContentLength: contentLen,
		Request:       req,
	}

	return resp, nil
}

// requestBody reads response data from a buffered channel and ensures
// the underlying C request is cleaned up safely on Close.
type requestBody struct {
	dataCh        chan []byte
	completeCh    chan struct{} // closed when OnCompleted fires
	cancelCh      chan struct{} // closed by Close() to cancel IO thread
	completionErr *error       // set before close(dataCh); read after
	buf           []byte       // remaining bytes from the last chunk
	req           *capi.Request
	handleID      uint64
	closed        bool
	mu            sync.Mutex
}

func (rb *requestBody) Read(p []byte) (int, error) {
	// Drain leftover bytes from the previous channel receive.
	if len(rb.buf) > 0 {
		n := copy(p, rb.buf)
		rb.buf = rb.buf[n:]
		return n, nil
	}

	data, ok := <-rb.dataCh
	if !ok {
		// Channel closed: OnCompleted has fired.
		// completionErr was written before close(dataCh), so safe to read.
		if rb.completionErr != nil && *rb.completionErr != nil {
			return 0, *rb.completionErr
		}
		return 0, io.EOF
	}

	n := copy(p, data)
	if n < len(data) {
		rb.buf = data[n:]
	}
	return n, nil
}

func (rb *requestBody) Close() error {
	rb.mu.Lock()
	if rb.closed {
		rb.mu.Unlock()
		return nil
	}
	rb.closed = true
	rb.mu.Unlock()

	// Signal the IO thread's data callback to stop (unblocks the select
	// in OnDataReceived if it's waiting on a full channel).
	select {
	case <-rb.cancelCh:
		// already closed
	default:
		close(rb.cancelCh)
	}

	// Wait for the request to fully complete on the IO thread.
	// After cancelCh is closed, the data callback returns cancel,
	// which triggers OnRequestComplete → OnCompleted → close(completeCh).
	<-rb.completeCh

	// Drain any remaining buffered data so the channel is empty.
	for range rb.dataCh {
	}

	// Now safe: the IO thread is done with this request.
	capi.UnregisterRequestHandle(rb.handleID)
	if rb.req != nil {
		capi.RequestDestroy(rb.req)
		rb.req = nil
	}
	return nil
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
