/*
 * cronet_websocket.cc -- WebSocket: WebSocketEventInterface implementation.
 */
#include "net/tools/cronet/cronet_websocket.h"

#include <tuple>

#include "base/functional/bind.h"
#include "base/task/single_thread_task_runner.h"
#include "net/base/io_buffer.h"
#include "net/base/net_errors.h"
#include "net/cookies/site_for_cookies.h"
#include "net/storage_access_api/status.h"
#include "net/traffic_annotation/network_traffic_annotation.h"
#include "net/websockets/websocket_channel.h"
#include "url/gurl.h"
#include "url/origin.h"
#include "net/tools/cronet/cronet_engine.h"

namespace {
constexpr net::NetworkTrafficAnnotationTag kTrafficAnnotation =
    net::DefineNetworkTrafficAnnotation("cronet_websocket", R"()");
}  // namespace

CronetWebSocket::CronetWebSocket(CronetEngine* engine,
                                 const CronetWebSocketCallbacks* callbacks,
                                 void* user_data)
    : engine_(engine), user_data_(user_data) {
  if (callbacks) {
    callbacks_ = *callbacks;
  } else {
    memset(&callbacks_, 0, sizeof(callbacks_));
  }
}

CronetWebSocket::~CronetWebSocket() {
  if (channel_) {
    base::WaitableEvent done;
    engine_->io_thread()->task_runner()->PostTask(
        FROM_HERE,
        base::BindOnce(&CronetWebSocket::DestroyOnIOThread,
                       base::Unretained(this)));
    // Note: channel_ will be destroyed on IO thread via destructor ordering
    // since we're stopping the thread in CronetEngine::~CronetEngine.
  }
}

void CronetWebSocket::AddHeader(const char* name, const char* value) {
  if (name && value) {
    extra_headers_.SetHeader(name, value);
  }
}

void CronetWebSocket::Connect(const char* url,
                               const char* const* subprotocols,
                               int num_subprotocols) {
  std::string url_str(url ? url : "");
  std::vector<std::string> protocols;
  if (subprotocols) {
    for (int i = 0; i < num_subprotocols; ++i) {
      if (subprotocols[i]) {
        protocols.emplace_back(subprotocols[i]);
      }
    }
  }

  engine_->io_thread()->task_runner()->PostTask(
      FROM_HERE, base::BindOnce(&CronetWebSocket::ConnectOnIOThread,
                                base::Unretained(this), std::move(url_str),
                                std::move(protocols)));
}

void CronetWebSocket::Send(const char* data, size_t length, int is_binary) {
  std::string payload(data ? data : "", data ? length : 0);
  engine_->io_thread()->task_runner()->PostTask(
      FROM_HERE, base::BindOnce(&CronetWebSocket::SendOnIOThread,
                                base::Unretained(this), std::move(payload),
                                is_binary));
}

void CronetWebSocket::Close(uint16_t code, const char* reason) {
  std::string reason_str(reason ? reason : "");
  engine_->io_thread()->task_runner()->PostTask(
      FROM_HERE, base::BindOnce(&CronetWebSocket::CloseOnIOThread,
                                base::Unretained(this), code,
                                std::move(reason_str)));
}

// -- IO thread methods --

void CronetWebSocket::ConnectOnIOThread(
    const std::string& url,
    std::vector<std::string> subprotocols) {
  channel_ = std::make_unique<net::WebSocketChannel>(
      std::unique_ptr<net::WebSocketEventInterface>(this),
      engine_->context());

  GURL gurl(url);
  url::Origin origin = url::Origin::Create(gurl);
  net::SiteForCookies site_for_cookies =
      net::SiteForCookies::FromOrigin(origin);
  net::IsolationInfo isolation_info =
      net::IsolationInfo::CreateTransient(std::nullopt);

  channel_->SendAddChannelRequest(
      gurl, subprotocols, origin, site_for_cookies,
      net::StorageAccessApiStatus::kNone, isolation_info, extra_headers_,
      kTrafficAnnotation);
}

void CronetWebSocket::SendOnIOThread(std::string data, int is_binary) {
  if (!channel_) return;

  auto buffer = base::MakeRefCounted<net::IOBufferWithSize>(data.size());
  if (!data.empty()) {
    memcpy(buffer->data(), data.data(), data.size());
  }

  net::WebSocketEventInterface::WebSocketMessageType opcode =
      is_binary ? net::WebSocketEventInterface::WebSocketMessageType(2)
                : net::WebSocketEventInterface::WebSocketMessageType(1);

  std::ignore = channel_->SendFrame(true, opcode, std::move(buffer),
                                    data.size());
}

void CronetWebSocket::CloseOnIOThread(uint16_t code,
                                       const std::string& reason) {
  if (channel_) {
    std::ignore = channel_->StartClosingHandshake(code, reason);
  }
}

void CronetWebSocket::DestroyOnIOThread() {
  channel_.reset();
}

// -- WebSocketEventInterface overrides --

void CronetWebSocket::OnCreateURLRequest(net::URLRequest* request) {
  // No custom setup needed.
}

int CronetWebSocket::OnURLRequestConnected(
    net::URLRequest* request,
    const net::TransportInfo& info,
    net::CompletionOnceCallback callback) {
  return net::OK;
}

void CronetWebSocket::OnAddChannelResponse(
    std::unique_ptr<net::WebSocketHandshakeResponseInfo> response,
    const std::string& selected_subprotocol,
    const std::string& extensions) {
  if (callbacks_.on_connected) {
    callbacks_.on_connected(this, selected_subprotocol.c_str(), user_data_);
  }
}

void CronetWebSocket::OnDataFrame(bool fin,
                                   WebSocketMessageType type,
                                   base::span<const char> payload) {
  // Type is 1 = text, 2 = binary, 0 = continuation
  if (type != 0) {
    // Start of a new message
    current_message_type_ = type;
    message_buffer_.clear();
  }

  message_buffer_.append(payload.data(), payload.size());

  if (fin) {
    // Complete message received
    if (callbacks_.on_message) {
      int is_binary = (current_message_type_ == 2) ? 1 : 0;
      callbacks_.on_message(this, message_buffer_.data(),
                           message_buffer_.size(), is_binary, user_data_);
    }
    message_buffer_.clear();
  }
}

bool CronetWebSocket::HasPendingDataFrames() {
  return false;
}

void CronetWebSocket::OnSendDataFrameDone() {
  // Send completed; could add a callback here if needed.
}

void CronetWebSocket::OnClosingHandshake() {
  // Server initiated close; wait for OnDropChannel.
}

void CronetWebSocket::OnDropChannel(bool was_clean,
                                     uint16_t code,
                                     const std::string& reason) {
  if (callbacks_.on_closed) {
    callbacks_.on_closed(this, was_clean ? 1 : 0, code,
                        reason.empty() ? nullptr : reason.c_str(), user_data_);
  }
}

void CronetWebSocket::OnFailChannel(const std::string& message,
                                     int net_error,
                                     std::optional<int> response_code) {
  if (callbacks_.on_error) {
    callbacks_.on_error(this, net_error,
                       message.empty() ? nullptr : message.c_str(),
                       user_data_);
  }
}

void CronetWebSocket::OnStartOpeningHandshake(
    std::unique_ptr<net::WebSocketHandshakeRequestInfo> request) {
  // Handshake in progress; no user callback needed.
}

void CronetWebSocket::OnSSLCertificateError(
    std::unique_ptr<net::WebSocketEventInterface::SSLErrorCallbacks>
        ssl_error_callbacks,
    const GURL& url,
    int net_error,
    const net::SSLInfo& ssl_info,
    bool fatal) {
  // If engine has insecure_skip_verify, accept the certificate.
  if (engine_->insecure_skip_verify()) {
    ssl_error_callbacks->ContinueSSLRequest();
    return;
  }
  // Otherwise reject invalid certificates.
  ssl_error_callbacks->CancelSSLRequest(net_error, &ssl_info);
}

int CronetWebSocket::OnAuthRequired(
    const net::AuthChallengeInfo& auth_info,
    scoped_refptr<net::HttpResponseHeaders> response_headers,
    const net::IPEndPoint& host_port_pair,
    base::OnceCallback<void(const net::AuthCredentials*)> callback,
    std::optional<net::AuthCredentials>* credentials) {
  // No auth support for WebSocket connections.
  return net::ERR_FAILED;
}
