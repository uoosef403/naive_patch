// Package cronet provides Go bindings for libcronet, Chromium's network stack
// as a standalone shared library with Chrome-identical TLS fingerprint.
package cronet

import (
	"fmt"

	"go-cronet/internal/capi"
)

// NetError represents a Chromium network stack error.
type NetError struct {
	Code    int
	Message string
}

func (e *NetError) Error() string {
	return fmt.Sprintf("cronet: %s (code %d)", e.Message, e.Code)
}

// NewNetError creates a NetError from a Chromium net error code.
// Returns nil if code == 0 (success).
func NewNetError(code int) error {
	if code == 0 {
		return nil
	}
	return &NetError{
		Code:    code,
		Message: capi.ErrorToString(code),
	}
}

// Common Chromium net error codes.
const (
	ErrIOPending                  = -1
	ErrFailed                     = -2
	ErrAborted                    = -3
	ErrInvalidArgument            = -4
	ErrInvalidHandle              = -5
	ErrFileNotFound               = -6
	ErrTimedOut                   = -7
	ErrFileTooBig                 = -8
	ErrUnexpected                 = -9
	ErrAccessDenied               = -10
	ErrNotImplemented             = -11
	ErrInsufficientResources      = -12
	ErrConnectionClosed           = -100
	ErrConnectionReset            = -101
	ErrConnectionRefused          = -102
	ErrConnectionAborted          = -103
	ErrConnectionFailed           = -104
	ErrNameNotResolved            = -105
	ErrInternetDisconnected       = -106
	ErrSSLProtocolError           = -107
	ErrAddressInvalid             = -108
	ErrAddressUnreachable         = -109
	ErrSSLClientAuthCertNeeded    = -110
	ErrTunnelConnectionFailed     = -111
	ErrSSLCertificateError        = -200
	ErrSSLCertDateInvalid         = -201
	ErrSSLCertAuthorityInvalid    = -202
	ErrSSLCertCommonNameInvalid   = -200
	ErrSSLCertRevoked             = -206
	ErrSSLCertInvalid             = -207
	ErrSSLVersionOrCipherMismatch = -113
	ErrTooManyRedirects           = -310
)

// Priority represents a request priority level.
type Priority int

const (
	PriorityDefault Priority = 0
	PriorityIdle    Priority = 1
	PriorityLowest  Priority = 2
	PriorityLow     Priority = 3
	PriorityMedium  Priority = 4
	PriorityHighest Priority = 5
)
