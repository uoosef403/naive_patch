/*
 * cronet_api.h -- Public C API for libcronet.so
 *
 * A Chrome-fingerprint-identical HTTP library built from Chromium's net stack.
 * Use from C, Zig (@cImport), Rust (bindgen), or Go (cgo).
 */
#ifndef CRONET_API_H_
#define CRONET_API_H_

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

#if defined(CRONET_BUILDING_SHARED)
  #if defined(_WIN32)
    #define CRONET_EXPORT __declspec(dllexport)
  #else
    #define CRONET_EXPORT __attribute__((visibility("default")))
  #endif
#elif defined(_WIN32) && !defined(CRONET_STATIC)
  #define CRONET_EXPORT __declspec(dllimport)
#else
  #define CRONET_EXPORT
#endif

/* -------------------------------------------------------------------------- */
/*  Constants                                                                  */
/* -------------------------------------------------------------------------- */

/* TLS version constants for CronetEngineParams.tls_min/max_version */
#define CRONET_TLS_VERSION_DEFAULT  0
#define CRONET_TLS_VERSION_TLS_1_0  0x0301
#define CRONET_TLS_VERSION_TLS_1_1  0x0302
#define CRONET_TLS_VERSION_TLS_1_2  0x0303
#define CRONET_TLS_VERSION_TLS_1_3  0x0304

/* Request priority constants for CronetRequestParams.priority */
#define CRONET_PRIORITY_DEFAULT  0
#define CRONET_PRIORITY_IDLE     1
#define CRONET_PRIORITY_LOWEST   2
#define CRONET_PRIORITY_LOW      3
#define CRONET_PRIORITY_MEDIUM   4
#define CRONET_PRIORITY_HIGHEST  5

/* -------------------------------------------------------------------------- */
/*  Opaque handle types                                                       */
/* -------------------------------------------------------------------------- */

typedef struct CronetEngine CronetEngine;
typedef struct CronetRequest CronetRequest;
typedef struct CronetWebSocket CronetWebSocket;

/* -------------------------------------------------------------------------- */
/*  Engine configuration                                                      */
/* -------------------------------------------------------------------------- */

typedef struct CronetEngineParams {
    const char* user_agent;          /* NULL = Chromium default */
    int         enable_quic;         /* 1 = enable QUIC/HTTP3, 0 = HTTP2 only */
    int         enable_http2;        /* 1 = enable HTTP/2 (default) */
    int         enable_brotli;       /* 1 = enable Brotli decompression */
    int         enable_zstd;         /* 1 = enable Zstd decompression */
    const char* proxy_url;           /* NULL = direct, e.g. "http://proxy:8080" */
    const char* cache_path;          /* NULL = in-memory only */
    int         cache_max_bytes;     /* 0 = default */

    /* --- TLS configuration --- */
    int         insecure_skip_verify;  /* 1 = accept any TLS certificate */
    int         enforce_strict_security; /* 1 = enable strict security (SameSite, Secure cookies). 0 = permissive (default). */
    const char* custom_ca_pem;         /* NULL or PEM-encoded root CA bundle */
    int         tls_min_version;       /* 0 = default. Use CRONET_TLS_VERSION_* */
    int         tls_max_version;       /* 0 = default. Use CRONET_TLS_VERSION_* */

    /* --- Cookie configuration --- */
    int         enable_cookies;        /* 1 = enable in-memory cookie jar */
} CronetEngineParams;

/* -------------------------------------------------------------------------- */
/*  Per-request parameters (optional, call set_params before start)           */
/* -------------------------------------------------------------------------- */

typedef struct CronetRequestParams {
    int         timeout_ms;            /* 0 = no timeout. Overall request timeout */
    int         connect_timeout_ms;    /* 0 = no connect timeout */
    int         max_redirects;         /* -1 = default (20), 0 = no auto-follow */
    int         priority;              /* CRONET_PRIORITY_*. 0 = DEFAULT */
    int         disable_cache;         /* 1 = bypass cache for this request */
} CronetRequestParams;

/* -------------------------------------------------------------------------- */
/*  TLS connection info (populated after on_response_started)                 */
/* -------------------------------------------------------------------------- */

typedef struct CronetTlsInfo {
    uint16_t    cipher_suite;          /* IANA cipher suite ID */
    uint16_t    tls_version;           /* 0x0303=TLS1.2, 0x0304=TLS1.3, etc. */
    const char* negotiated_protocol;   /* "h2", "h3", "http/1.1" */
    int         cert_chain_count;      /* Certs in chain (access via get_tls_cert_pem) */
} CronetTlsInfo;

/* -------------------------------------------------------------------------- */
/*  Request timing info (populated after on_completed)                        */
/* -------------------------------------------------------------------------- */

typedef struct CronetTimingInfo {
    int64_t dns_start_ms;              /* Milliseconds from request start */
    int64_t dns_end_ms;
    int64_t connect_start_ms;
    int64_t connect_end_ms;
    int64_t ssl_start_ms;
    int64_t ssl_end_ms;
    int64_t send_start_ms;
    int64_t send_end_ms;
    int64_t first_byte_ms;            /* Time to first response byte */
    int64_t completion_ms;            /* Total time */
} CronetTimingInfo;

/* -------------------------------------------------------------------------- */
/*  HTTP request callbacks                                                    */
/* -------------------------------------------------------------------------- */

typedef struct CronetRequestCallbacks {
    /* Called when response headers arrive.
     * Return 0 to continue, non-zero to cancel. */
    int  (*on_response_started)(CronetRequest* req, int status_code,
                                const char* status_text, void* user_data);

    /* Called for each chunk of response body.
     * |data| is valid only for the duration of this call.
     * Return 0 to continue reading, non-zero to cancel. */
    int  (*on_data_received)(CronetRequest* req, const char* data,
                             size_t length, void* user_data);

    /* Called when the request completes (success, error, or cancel).
     * |net_error| is 0 on success, negative Chromium net error code otherwise. */
    void (*on_completed)(CronetRequest* req, int net_error, void* user_data);

    /* Called on redirect. Return 0 to follow, non-zero to cancel.
     * |new_url| is the redirect target. May be NULL to auto-follow all. */
    int  (*on_redirect)(CronetRequest* req, int status_code,
                        const char* new_url, void* user_data);
} CronetRequestCallbacks;

/* -------------------------------------------------------------------------- */
/*  WebSocket callbacks                                                       */
/* -------------------------------------------------------------------------- */

typedef struct CronetWebSocketCallbacks {
    /* Called when the WebSocket connection is established. */
    void (*on_connected)(CronetWebSocket* ws, const char* selected_subprotocol,
                         void* user_data);

    /* Called when a complete message is received.
     * |is_binary| indicates text (0) or binary (1) message type.
     * |data| is valid only for the duration of this call. */
    void (*on_message)(CronetWebSocket* ws, const char* data, size_t length,
                       int is_binary, void* user_data);

    /* Called when the WebSocket is closed (cleanly or not).
     * |code| is the WebSocket close code, |reason| may be NULL. */
    void (*on_closed)(CronetWebSocket* ws, int was_clean, uint16_t code,
                      const char* reason, void* user_data);

    /* Called on connection failure or protocol error.
     * |net_error| is a Chromium net error code. */
    void (*on_error)(CronetWebSocket* ws, int net_error,
                     const char* message, void* user_data);
} CronetWebSocketCallbacks;

/* -------------------------------------------------------------------------- */
/*  Version                                                                   */
/* -------------------------------------------------------------------------- */

CRONET_EXPORT const char* cronet_version(void);
CRONET_EXPORT const char* cronet_user_agent_string(void);

/* -------------------------------------------------------------------------- */
/*  Engine lifecycle                                                          */
/* -------------------------------------------------------------------------- */

CRONET_EXPORT CronetEngine* cronet_engine_create(const CronetEngineParams* params);
CRONET_EXPORT void          cronet_engine_destroy(CronetEngine* engine);

/* -------------------------------------------------------------------------- */
/*  Cookie management (engine-level, shared across requests)                  */
/* -------------------------------------------------------------------------- */

/* Set a cookie. |cookie_line| is in Set-Cookie format. |url| is the scope. */
CRONET_EXPORT void cronet_engine_set_cookie(CronetEngine* engine,
                                             const char* url,
                                             const char* cookie_line);

/* Get all cookies for a URL. Returns semicolon-separated "name=value" string.
 * Caller MUST free the returned string with cronet_free_string(). */
CRONET_EXPORT char* cronet_engine_get_cookies(CronetEngine* engine,
                                               const char* url);

/* Delete all cookies. */
CRONET_EXPORT void cronet_engine_clear_cookies(CronetEngine* engine);

/* -------------------------------------------------------------------------- */
/*  HTTP request lifecycle                                                    */
/* -------------------------------------------------------------------------- */

CRONET_EXPORT CronetRequest* cronet_request_create(
    CronetEngine* engine,
    const char* method,                     /* "GET", "POST", etc. */
    const char* url,
    const CronetRequestCallbacks* callbacks,
    void* user_data
);

CRONET_EXPORT void cronet_request_add_header(CronetRequest* req,
                                             const char* name,
                                             const char* value);

CRONET_EXPORT void cronet_request_set_body(CronetRequest* req,
                                           const char* content_type,
                                           const char* data,
                                           size_t length);

/* Set per-request parameters. Call before cronet_request_start(). */
CRONET_EXPORT void cronet_request_set_params(CronetRequest* req,
                                              const CronetRequestParams* params);

CRONET_EXPORT void cronet_request_start(CronetRequest* req);
CRONET_EXPORT void cronet_request_cancel(CronetRequest* req);
CRONET_EXPORT void cronet_request_destroy(CronetRequest* req);

/* Block until the request completes. Returns net_error (0 = OK).
 * |timeout_ms| <= 0 means wait indefinitely.
 * Do NOT call from within a Cronet callback (deadlock). */
CRONET_EXPORT int cronet_request_wait(CronetRequest* req, int timeout_ms);

/* Response inspection (valid after on_response_started) */
CRONET_EXPORT int         cronet_request_get_status_code(CronetRequest* req);
CRONET_EXPORT const char* cronet_request_get_header(CronetRequest* req,
                                                     const char* name);
CRONET_EXPORT int         cronet_request_get_header_count(CronetRequest* req);
CRONET_EXPORT const char* cronet_request_get_header_name(CronetRequest* req,
                                                          int index);
CRONET_EXPORT const char* cronet_request_get_header_value(CronetRequest* req,
                                                           int index);
CRONET_EXPORT const char* cronet_request_get_negotiated_protocol(CronetRequest* req);

/* Final URL after all redirects (valid after on_response_started). */
CRONET_EXPORT const char* cronet_request_get_url(CronetRequest* req);

/* Response Content-Length, or -1 if unknown. */
CRONET_EXPORT int64_t cronet_request_get_content_length(CronetRequest* req);

/* HTTP version string: "HTTP/1.0", "HTTP/1.1", "HTTP/2", "HTTP/3". */
CRONET_EXPORT const char* cronet_request_get_http_version(CronetRequest* req);

/* TLS connection info. Returns 0 on success, -1 if not TLS. */
CRONET_EXPORT int cronet_request_get_tls_info(CronetRequest* req,
                                               CronetTlsInfo* out);

/* PEM-encoded certificate at |index| in the peer's cert chain.
 * Returns NULL if index is out of range. Pointer valid until request destroyed. */
CRONET_EXPORT const char* cronet_request_get_tls_cert_pem(CronetRequest* req,
                                                           int index);

/* Request timing (valid after on_completed). Returns 0 on success, -1 if N/A. */
CRONET_EXPORT int cronet_request_get_timing(CronetRequest* req,
                                             CronetTimingInfo* out);

/* -------------------------------------------------------------------------- */
/*  WebSocket lifecycle                                                       */
/* -------------------------------------------------------------------------- */

CRONET_EXPORT CronetWebSocket* cronet_websocket_create(
    CronetEngine* engine,
    const CronetWebSocketCallbacks* callbacks,
    void* user_data
);

CRONET_EXPORT void cronet_websocket_add_header(CronetWebSocket* ws,
                                                const char* name,
                                                const char* value);

CRONET_EXPORT void cronet_websocket_connect(CronetWebSocket* ws,
                                             const char* url,
                                             const char* const* subprotocols,
                                             int num_subprotocols);

CRONET_EXPORT void cronet_websocket_send(CronetWebSocket* ws,
                                          const char* data,
                                          size_t length,
                                          int is_binary);

CRONET_EXPORT void cronet_websocket_close(CronetWebSocket* ws,
                                           uint16_t code,
                                           const char* reason);

CRONET_EXPORT void cronet_websocket_destroy(CronetWebSocket* ws);

/* -------------------------------------------------------------------------- */
/*  Utility                                                                   */
/* -------------------------------------------------------------------------- */

/* Convert a Chromium net error code to a human-readable string.
 * Returns pointer to thread-local storage (no need to free). */
CRONET_EXPORT const char* cronet_error_to_string(int net_error);

/* Free a string allocated by the library (e.g. from cronet_engine_get_cookies). */
CRONET_EXPORT void cronet_free_string(char* str);

#ifdef __cplusplus
}  /* extern "C" */
#endif

#endif  /* CRONET_API_H_ */
