/*
 * cronet_export.cc -- extern "C" exported function bodies.
 *
 * All CRONET_EXPORT functions delegate to internal C++ structs.
 */
#include "net/tools/cronet/cronet_api.h"
#include "net/tools/cronet/cronet_engine.h"
#include "net/tools/cronet/cronet_request.h"
#include "net/tools/cronet/cronet_websocket.h"

#include <cstdlib>
#include <string>

#include "base/version_info/version_info.h"
#include "net/base/net_errors.h"

/* -------------------------------------------------------------------------- */
/*  Version                                                                   */
/* -------------------------------------------------------------------------- */

extern "C" CRONET_EXPORT const char* cronet_version(void) {
  static std::string version =
      std::string(version_info::GetVersionNumber());
  return version.c_str();
}

extern "C" CRONET_EXPORT const char* cronet_user_agent_string(void) {
  static std::string ua =
      "Cronet/" + std::string(version_info::GetVersionNumber());
  return ua.c_str();
}

/* -------------------------------------------------------------------------- */
/*  Engine lifecycle                                                          */
/* -------------------------------------------------------------------------- */

extern "C" CRONET_EXPORT CronetEngine* cronet_engine_create(
    const CronetEngineParams* params) {
  auto* engine = new CronetEngine();
  engine->Init(params);
  return engine;
}

extern "C" CRONET_EXPORT void cronet_engine_destroy(CronetEngine* engine) {
  delete engine;
}

/* -------------------------------------------------------------------------- */
/*  Cookie management                                                         */
/* -------------------------------------------------------------------------- */

extern "C" CRONET_EXPORT void cronet_engine_set_cookie(
    CronetEngine* engine,
    const char* url,
    const char* cookie_line) {
  if (engine) engine->SetCookie(url, cookie_line);
}

extern "C" CRONET_EXPORT char* cronet_engine_get_cookies(
    CronetEngine* engine,
    const char* url) {
  return engine ? engine->GetCookies(url) : nullptr;
}

extern "C" CRONET_EXPORT void cronet_engine_clear_cookies(
    CronetEngine* engine) {
  if (engine) engine->ClearCookies();
}

/* -------------------------------------------------------------------------- */
/*  HTTP request lifecycle                                                    */
/* -------------------------------------------------------------------------- */

extern "C" CRONET_EXPORT CronetRequest* cronet_request_create(
    CronetEngine* engine,
    const char* method,
    const char* url,
    const CronetRequestCallbacks* callbacks,
    void* user_data) {
  if (!engine) return nullptr;
  return new CronetRequest(engine, method, url, callbacks, user_data);
}

extern "C" CRONET_EXPORT void cronet_request_add_header(CronetRequest* req,
                                                         const char* name,
                                                         const char* value) {
  if (req) req->AddHeader(name, value);
}

extern "C" CRONET_EXPORT void cronet_request_set_body(CronetRequest* req,
                                                       const char* content_type,
                                                       const char* data,
                                                       size_t length) {
  if (req) req->SetBody(content_type, data, length);
}

extern "C" CRONET_EXPORT void cronet_request_set_params(
    CronetRequest* req,
    const CronetRequestParams* params) {
  if (req) req->SetParams(params);
}

extern "C" CRONET_EXPORT void cronet_request_start(CronetRequest* req) {
  if (req) req->Start();
}

extern "C" CRONET_EXPORT void cronet_request_cancel(CronetRequest* req) {
  if (req) req->Cancel();
}

extern "C" CRONET_EXPORT void cronet_request_destroy(CronetRequest* req) {
  delete req;
}

extern "C" CRONET_EXPORT int cronet_request_wait(CronetRequest* req,
                                                  int timeout_ms) {
  return req ? req->Wait(timeout_ms) : -1;
}

extern "C" CRONET_EXPORT int cronet_request_get_status_code(
    CronetRequest* req) {
  return req ? req->status_code() : 0;
}

extern "C" CRONET_EXPORT const char* cronet_request_get_header(
    CronetRequest* req,
    const char* name) {
  return req ? req->GetResponseHeader(name) : nullptr;
}

extern "C" CRONET_EXPORT int cronet_request_get_header_count(
    CronetRequest* req) {
  return req ? req->GetResponseHeaderCount() : 0;
}

extern "C" CRONET_EXPORT const char* cronet_request_get_header_name(
    CronetRequest* req,
    int index) {
  return req ? req->GetResponseHeaderName(index) : nullptr;
}

extern "C" CRONET_EXPORT const char* cronet_request_get_header_value(
    CronetRequest* req,
    int index) {
  return req ? req->GetResponseHeaderValue(index) : nullptr;
}

extern "C" CRONET_EXPORT const char* cronet_request_get_negotiated_protocol(
    CronetRequest* req) {
  return req ? req->GetNegotiatedProtocol() : nullptr;
}

extern "C" CRONET_EXPORT const char* cronet_request_get_url(
    CronetRequest* req) {
  return req ? req->GetUrl() : nullptr;
}

extern "C" CRONET_EXPORT int64_t cronet_request_get_content_length(
    CronetRequest* req) {
  return req ? req->GetContentLength() : -1;
}

extern "C" CRONET_EXPORT const char* cronet_request_get_http_version(
    CronetRequest* req) {
  return req ? req->GetHttpVersion() : nullptr;
}

extern "C" CRONET_EXPORT int cronet_request_get_tls_info(
    CronetRequest* req,
    CronetTlsInfo* out) {
  return req ? req->GetTlsInfo(out) : -1;
}

extern "C" CRONET_EXPORT const char* cronet_request_get_tls_cert_pem(
    CronetRequest* req,
    int index) {
  return req ? req->GetTlsCertPem(index) : nullptr;
}

extern "C" CRONET_EXPORT int cronet_request_get_timing(
    CronetRequest* req,
    CronetTimingInfo* out) {
  return req ? req->GetTiming(out) : -1;
}

/* -------------------------------------------------------------------------- */
/*  WebSocket lifecycle                                                       */
/* -------------------------------------------------------------------------- */

extern "C" CRONET_EXPORT CronetWebSocket* cronet_websocket_create(
    CronetEngine* engine,
    const CronetWebSocketCallbacks* callbacks,
    void* user_data) {
  if (!engine) return nullptr;
  return new CronetWebSocket(engine, callbacks, user_data);
}

extern "C" CRONET_EXPORT void cronet_websocket_add_header(
    CronetWebSocket* ws,
    const char* name,
    const char* value) {
  if (ws) ws->AddHeader(name, value);
}

extern "C" CRONET_EXPORT void cronet_websocket_connect(
    CronetWebSocket* ws,
    const char* url,
    const char* const* subprotocols,
    int num_subprotocols) {
  if (ws) ws->Connect(url, subprotocols, num_subprotocols);
}

extern "C" CRONET_EXPORT void cronet_websocket_send(CronetWebSocket* ws,
                                                     const char* data,
                                                     size_t length,
                                                     int is_binary) {
  if (ws) ws->Send(data, length, is_binary);
}

extern "C" CRONET_EXPORT void cronet_websocket_close(CronetWebSocket* ws,
                                                      uint16_t code,
                                                      const char* reason) {
  if (ws) ws->Close(code, reason);
}

extern "C" CRONET_EXPORT void cronet_websocket_destroy(CronetWebSocket* ws) {
  delete ws;
}

/* -------------------------------------------------------------------------- */
/*  Utility                                                                   */
/* -------------------------------------------------------------------------- */

extern "C" CRONET_EXPORT const char* cronet_error_to_string(int net_error) {
  thread_local std::string tl_buf;
  tl_buf = net::ErrorToString(net_error);
  return tl_buf.c_str();
}

extern "C" CRONET_EXPORT void cronet_free_string(char* str) {
  free(str);
}
