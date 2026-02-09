package cronet

import (
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"time"
)

// TLSInfo contains TLS connection details.
type TLSInfo struct {
	Version     uint16                // TLS version (0x0303=TLS1.2, 0x0304=TLS1.3)
	CipherSuite uint16               // IANA cipher suite ID
	Protocol    string                // ALPN negotiated protocol ("h2", "h3", "http/1.1")
	PeerCerts   []*x509.Certificate   // Peer certificate chain
	PeerCertsPEM []string             // PEM-encoded certificates
}

// TimingInfo contains request timing details.
type TimingInfo struct {
	DNSLookup    time.Duration
	Connect      time.Duration
	TLSHandshake time.Duration
	SendRequest  time.Duration
	FirstByte    time.Duration
	Total        time.Duration
}

// Response extends http.Response with Chromium-specific data.
type Response struct {
	*http.Response

	// Chromium-specific fields
	TLS      *TLSInfo
	Timing   *TimingInfo
	FinalURL string // URL after redirects
	Proto    string // "HTTP/1.1", "HTTP/2", "HTTP/3"
}

// parsePEMCerts parses PEM strings into x509 certificates.
func parsePEMCerts(pems []string) []*x509.Certificate {
	var certs []*x509.Certificate
	for _, p := range pems {
		block, _ := pem.Decode([]byte(p))
		if block == nil {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err == nil {
			certs = append(certs, cert)
		}
	}
	return certs
}
