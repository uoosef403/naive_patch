/*
 * cronet_export.cc -- extern "C" exported function bodies.
 *
 * All CRONET_EXPORT functions delegate to internal C++ structs.
 */
#include "tools/cronet/cronet_api.h"
#include "tools/cronet/cronet_engine.h"
#include "tools/cronet/cronet_request.h"
#include "tools/cronet/cronet_websocket.h"

#include "base/version_info/version_info.h"
#include "components/version_info/version_info_with_user_agent.h"

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
      std::string(version_info::GetUserAgent());
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

extern "C" CRONET_EXPORT void cronet_request_start(CronetRequest* req) {
  if (req) req->Start();
}

extern "C" CRONET_EXPORT void cronet_request_cancel(CronetRequest* req) {
  if (req) req->Cancel();
}

extern "C" CRONET_EXPORT void cronet_request_destroy(CronetRequest* req) {
  delete req;
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
