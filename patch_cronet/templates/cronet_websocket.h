/*
 * cronet_websocket.h -- Internal WebSocket struct.
 */
#ifndef NET_TOOLS_CRONET_CRONET_WEBSOCKET_H_
#define NET_TOOLS_CRONET_CRONET_WEBSOCKET_H_

#include <memory>
#include <string>
#include <vector>

#include "base/synchronization/waitable_event.h"
#include "net/http/http_request_headers.h"
#include "url/gurl.h"
#include "net/websockets/websocket_channel.h"
#include "net/websockets/websocket_event_interface.h"
#include "net/tools/cronet/cronet_api.h"

struct CronetEngine;
class CronetWebSocketAdapter;

struct CronetWebSocket {
  CronetWebSocket(CronetEngine* engine,
                  const CronetWebSocketCallbacks* callbacks,
                  void* user_data);
  ~CronetWebSocket();

  CronetWebSocket(const CronetWebSocket&) = delete;
  CronetWebSocket& operator=(const CronetWebSocket&) = delete;

  CronetEngine* engine() const { return engine_; }

  void AddHeader(const char* name, const char* value);
  void Connect(const char* url,
               const char* const* subprotocols,
               int num_subprotocols);
  void Send(const char* data, size_t length, int is_binary);
  void Close(uint16_t code, const char* reason);

  // Event handlers called by CronetWebSocketAdapter
  void OnConnected(const std::string& selected_subprotocol);
  void OnMessage(const std::string& data, int is_binary);
  void OnClosed(bool was_clean, uint16_t code, const std::string& reason);
  void OnError(int net_error, const std::string& message);
  void StartReading();

 private:
  void ConnectOnIOThread(const std::string& url,
                         std::vector<std::string> subprotocols);
  void SendOnIOThread(std::string data, int is_binary);
  void CloseOnIOThread(uint16_t code, const std::string& reason);
  void DestroyOnIOThread(base::WaitableEvent* done);
  void ReadFramesOnIOThread();

  CronetEngine* engine_;
  CronetWebSocketCallbacks callbacks_;
  void* user_data_;

  net::HttpRequestHeaders extra_headers_;
  std::unique_ptr<net::WebSocketChannel> channel_;
  CronetWebSocketAdapter* adapter_ = nullptr;  // Non-owning, for Detach()

  // Frame reassembly for multi-frame messages
  std::string message_buffer_;
  int current_message_type_ = 0;
};

#endif  // NET_TOOLS_CRONET_CRONET_WEBSOCKET_H_
