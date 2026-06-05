package cronet_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// testServer holds the shared test HTTPS + HTTP servers.
var testServer struct {
	once      sync.Once
	httpsAddr string // e.g. "127.0.0.1:14443"
	httpAddr  string // e.g. "127.0.0.1:14080"
	wsAddr    string // raw TCP WebSocket server (ws://)
	wssAddr   string // raw TLS WebSocket server (wss://)
	caPEM     string // PEM-encoded CA cert
	tlsCert   tls.Certificate
}

func setupTestServers(t *testing.T) {
	t.Helper()
	testServer.once.Do(func() {
		// Generate a self-signed CA and server certificate
		caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatalf("generate CA key: %v", err)
		}

		caTemplate := &x509.Certificate{
			SerialNumber: big.NewInt(1),
			Subject: pkix.Name{
				Organization: []string{"Cronet Test CA"},
			},
			NotBefore:             time.Now().Add(-time.Hour),
			NotAfter:              time.Now().Add(24 * time.Hour),
			KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
			BasicConstraintsValid: true,
			IsCA:                  true,
		}
		caCertDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
		if err != nil {
			t.Fatalf("create CA cert: %v", err)
		}
		caCert, err := x509.ParseCertificate(caCertDER)
		if err != nil {
			t.Fatalf("parse CA cert: %v", err)
		}
		caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCertDER})
		testServer.caPEM = string(caPEM)

		// Server key + cert signed by CA
		serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatalf("generate server key: %v", err)
		}
		serverTemplate := &x509.Certificate{
			SerialNumber: big.NewInt(2),
			Subject: pkix.Name{
				Organization: []string{"Cronet Test Server"},
			},
			DNSNames:    []string{"localhost", "127.0.0.1"},
			IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1)},
			NotBefore:   time.Now().Add(-time.Hour),
			NotAfter:    time.Now().Add(24 * time.Hour),
			KeyUsage:    x509.KeyUsageDigitalSignature,
			ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		}
		serverCertDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCert, &serverKey.PublicKey, caKey)
		if err != nil {
			t.Fatalf("create server cert: %v", err)
		}
		serverCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverCertDER})
		serverKeyDER, _ := x509.MarshalECPrivateKey(serverKey)
		serverKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: serverKeyDER})

		tlsCert, err := tls.X509KeyPair(serverCertPEM, serverKeyPEM)
		if err != nil {
			t.Fatalf("load TLS keypair: %v", err)
		}
		testServer.tlsCert = tlsCert

		// Set up HTTP mux with test routes
		mux := testMux()

		// Start HTTPS server (H1.1 + H2)
		httpsLn, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen HTTPS: %v", err)
		}
		testServer.httpsAddr = httpsLn.Addr().String()

		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{tlsCert},
			NextProtos:   []string{"h2", "http/1.1"},
		}
		tlsLn := tls.NewListener(httpsLn, tlsConfig)
		httpsServer := &http.Server{Handler: mux}
		go httpsServer.Serve(tlsLn)

		// Start HTTP server (cleartext)
		httpLn, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen HTTP: %v", err)
		}
		testServer.httpAddr = httpLn.Addr().String()
		httpServer := &http.Server{Handler: mux}
		go httpServer.Serve(httpLn)

		// Start raw TCP WebSocket server (ws://)
		wsLn, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen WS: %v", err)
		}
		testServer.wsAddr = wsLn.Addr().String()
		go func() {
			for {
				conn, err := wsLn.Accept()
				if err != nil {
					return
				}
				go handleWSConnRaw(conn)
			}
		}()

		// Start raw TLS WebSocket server (wss://)
		wssLn, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen WSS: %v", err)
		}
		wssTLSConfig := &tls.Config{
			Certificates: []tls.Certificate{tlsCert},
		}
		wssTLSLn := tls.NewListener(wssLn, wssTLSConfig)
		testServer.wssAddr = wssLn.Addr().String()
		go func() {
			for {
				conn, err := wssTLSLn.Accept()
				if err != nil {
					return
				}
				go handleWSConnRawTLS(conn)
			}
		}()

		// Write CA PEM to temp file for tests that need a path
		os.WriteFile("/tmp/cronet_test_ca.pem", caPEM, 0644)
	})
}

func testMux() *http.ServeMux {
	mux := http.NewServeMux()

	// /echo - echoes back request info as JSON
	mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		resp := map[string]interface{}{
			"method":  r.Method,
			"url":     r.URL.String(),
			"headers": flattenHeaders(r.Header),
			"body":    string(body),
			"proto":   r.Proto,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	// /status/{code} - returns specific status code
	mux.HandleFunc("/status/", func(w http.ResponseWriter, r *http.Request) {
		codeStr := strings.TrimPrefix(r.URL.Path, "/status/")
		var code int
		fmt.Sscanf(codeStr, "%d", &code)
		if code == 0 {
			code = 200
		}
		w.WriteHeader(code)
		fmt.Fprintf(w, "%d", code)
	})

	// /redirect/{n} - chain of n redirects ending at /echo
	mux.HandleFunc("/redirect/", func(w http.ResponseWriter, r *http.Request) {
		nStr := strings.TrimPrefix(r.URL.Path, "/redirect/")
		var n int
		fmt.Sscanf(nStr, "%d", &n)
		if n <= 1 {
			http.Redirect(w, r, "/echo", http.StatusFound)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/redirect/%d", n-1), http.StatusFound)
	})

	// /delay/{ms} - respond after delay
	mux.HandleFunc("/delay/", func(w http.ResponseWriter, r *http.Request) {
		msStr := strings.TrimPrefix(r.URL.Path, "/delay/")
		var ms int
		fmt.Sscanf(msStr, "%d", &ms)
		time.Sleep(time.Duration(ms) * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "delayed %dms", ms)
	})

	// /cookies/set?name=value - sets cookies via Set-Cookie header
	mux.HandleFunc("/cookies/set", func(w http.ResponseWriter, r *http.Request) {
		for key, values := range r.URL.Query() {
			for _, v := range values {
				http.SetCookie(w, &http.Cookie{
					Name:  key,
					Value: v,
					Path:  "/",
				})
			}
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "cookies set")
	})

	// /cookies/get - returns received cookies as JSON
	mux.HandleFunc("/cookies/get", func(w http.ResponseWriter, r *http.Request) {
		cookies := make(map[string]string)
		for _, c := range r.Cookies() {
			cookies[c.Name] = c.Value
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cookies)
	})

	// /large/{bytes} - returns N bytes of data
	mux.HandleFunc("/large/", func(w http.ResponseWriter, r *http.Request) {
		nStr := strings.TrimPrefix(r.URL.Path, "/large/")
		var n int
		fmt.Sscanf(nStr, "%d", &n)
		if n <= 0 {
			n = 1024
		}
		w.Header().Set("Content-Length", fmt.Sprintf("%d", n))
		// Write in chunks
		chunk := make([]byte, 8192)
		for i := range chunk {
			chunk[i] = byte('A' + (i % 26))
		}
		written := 0
		for written < n {
			toWrite := n - written
			if toWrite > len(chunk) {
				toWrite = len(chunk)
			}
			w.Write(chunk[:toWrite])
			written += toWrite
		}
	})

	// /tls-info - returns TLS info as seen by server
	mux.HandleFunc("/tls-info", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"proto": r.Proto,
		}
		if r.TLS != nil {
			resp["tls_version"] = r.TLS.Version
			resp["cipher_suite"] = r.TLS.CipherSuite
			resp["server_name"] = r.TLS.ServerName
			resp["negotiated_protocol"] = r.TLS.NegotiatedProtocol
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	// /headers - returns all received request headers as JSON
	mux.HandleFunc("/headers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(flattenHeaders(r.Header))
	})

	// /ws/echo - WebSocket echo server (RFC 6455, no external deps)
	mux.HandleFunc("/ws/echo", handleWebSocketEcho)

	return mux
}

func flattenHeaders(h http.Header) map[string]string {
	flat := make(map[string]string)
	for k, v := range h {
		flat[k] = strings.Join(v, ", ")
	}
	return flat
}

func httpsURL(path string) string {
	return "https://" + testServer.httpsAddr + path
}

func httpURL(path string) string {
	return "http://" + testServer.httpAddr + path
}

func wsURL(path string) string {
	return "ws://" + testServer.httpAddr + path
}

func wssURL(path string) string {
	return "wss://" + testServer.httpsAddr + path
}

func newRequest(method, url string, body io.Reader) (*http.Request, error) {
	return http.NewRequest(method, url, body)
}

func handleWebSocketEcho(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), time.Minute)
	defer cancel()

	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ws-echo] accept error: %v\n", err)
		return
	}
	defer conn.Close(websocket.StatusInternalError, "server error")

	fmt.Fprintf(os.Stderr, "[ws-echo] connection accepted\n")

	for {
		msgType, data, err := conn.Read(ctx)
		if err != nil {
			if websocket.CloseStatus(err) == websocket.StatusNormalClosure {
				fmt.Fprintf(os.Stderr, "[ws-echo] client closed normally\n")
				return
			}
			fmt.Fprintf(os.Stderr, "[ws-echo] read error: %v\n", err)
			return
		}

		fmt.Fprintf(os.Stderr, "[ws-echo] received %d bytes, type=%v\n", len(data), msgType)

		err = conn.Write(ctx, msgType, data)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[ws-echo] write error: %v\n", err)
			return
		}
		fmt.Fprintf(os.Stderr, "[ws-echo] echoed back\n")
	}
}

func handleWSConnRaw(conn net.Conn) {
	defer conn.Close()

	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
				http.Error(w, "not a websocket request", http.StatusBadRequest)
				return
			}
			handleWebSocketEcho(w, r)
		}),
		ReadTimeout:  time.Minute,
		WriteTimeout: time.Minute,
	}
	server.Serve(&singleConnListener{conn: conn})
}

func handleWSConnRawTLS(conn net.Conn) {
	handleWSConnRaw(conn)
}

type singleConnListener struct {
	conn net.Conn
	used bool
}

func (l *singleConnListener) Accept() (net.Conn, error) {
	if l.used {
		select {}
	}
	l.used = true
	return l.conn, nil
}

func (l *singleConnListener) Close() error   { return nil }
func (l *singleConnListener) Addr() net.Addr { return l.conn.LocalAddr() }
