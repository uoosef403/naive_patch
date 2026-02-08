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
} CronetEngineParams;

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

CRONET_EXPORT void cronet_request_start(CronetRequest* req);
CRONET_EXPORT void cronet_request_cancel(CronetRequest* req);
CRONET_EXPORT void cronet_request_destroy(CronetRequest* req);

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

#ifdef __cplusplus
}  /* extern "C" */
#endif

#endif  /* CRONET_API_H_ */
