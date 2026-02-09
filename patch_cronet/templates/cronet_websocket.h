/*
 * cronet_websocket.h -- Internal WebSocket struct.
 */
#ifndef NET_TOOLS_CRONET_CRONET_WEBSOCKET_H_
#define NET_TOOLS_CRONET_CRONET_WEBSOCKET_H_

#include <memory>
#include <string>
#include <vector>

#include "net/http/http_request_headers.h"
#include "url/gurl.h"
#include "net/websockets/websocket_channel.h"
#include "net/websockets/websocket_event_interface.h"
#include "net/tools/cronet/cronet_api.h"

struct CronetEngine;

struct CronetWebSocket : public net::WebSocketEventInterface {
  CronetWebSocket(CronetEngine* engine,
                  const CronetWebSocketCallbacks* callbacks,
                  void* user_data);
  ~CronetWebSocket() override;

  CronetWebSocket(const CronetWebSocket&) = delete;
  CronetWebSocket& operator=(const CronetWebSocket&) = delete;

  void AddHeader(const char* name, const char* value);
  void Connect(const char* url,
               const char* const* subprotocols,
               int num_subprotocols);
  void Send(const char* data, size_t length, int is_binary);
  void Close(uint16_t code, const char* reason);

  // WebSocketEventInterface overrides
  void OnCreateURLRequest(net::URLRequest* request) override;
  int OnURLRequestConnected(
      net::URLRequest* request,
      const net::TransportInfo& info,
      net::CompletionOnceCallback callback) override;
  void OnAddChannelResponse(
      std::unique_ptr<net::WebSocketHandshakeResponseInfo> response,
      const std::string& selected_subprotocol,
      const std::string& extensions) override;
  void OnDataFrame(bool fin,
                   WebSocketMessageType type,
                   base::span<const char> payload) override;
  bool HasPendingDataFrames() override;
  void OnSendDataFrameDone() override;
  void OnClosingHandshake() override;
  void OnDropChannel(bool was_clean,
                     uint16_t code,
                     const std::string& reason) override;
  void OnFailChannel(const std::string& message,
                     int net_error,
                     std::optional<int> response_code) override;
  void OnStartOpeningHandshake(
      std::unique_ptr<net::WebSocketHandshakeRequestInfo> request) override;
  void OnSSLCertificateError(
      std::unique_ptr<net::WebSocketEventInterface::SSLErrorCallbacks>
          ssl_error_callbacks,
      const GURL& url,
      int net_error,
      const net::SSLInfo& ssl_info,
      bool fatal) override;
  int OnAuthRequired(
      const net::AuthChallengeInfo& auth_info,
      scoped_refptr<net::HttpResponseHeaders> response_headers,
      const net::IPEndPoint& host_port_pair,
      base::OnceCallback<void(const net::AuthCredentials*)> callback,
      std::optional<net::AuthCredentials>* credentials) override;

 private:
  void ConnectOnIOThread(const std::string& url,
                         std::vector<std::string> subprotocols);
  void SendOnIOThread(std::string data, int is_binary);
  void CloseOnIOThread(uint16_t code, const std::string& reason);
  void DestroyOnIOThread();

  CronetEngine* engine_;
  CronetWebSocketCallbacks callbacks_;
  void* user_data_;

  net::HttpRequestHeaders extra_headers_;
  std::unique_ptr<net::WebSocketChannel> channel_;

  // Frame reassembly for multi-frame messages
  std::string message_buffer_;
  int current_message_type_ = 0;
};

#endif  // NET_TOOLS_CRONET_CRONET_WEBSOCKET_H_
