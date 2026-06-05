/*
 * cronet_websocket.cc -- WebSocket: WebSocketEventInterface implementation.
 */
#include "net/tools/cronet/cronet_websocket.h"

#include <tuple>

#include "base/functional/bind.h"
#include "base/task/single_thread_task_runner.h"
#include "base/synchronization/waitable_event.h"
#include "net/base/io_buffer.h"
#include "net/base/net_errors.h"
#include "net/cookies/site_for_cookies.h"
#include "net/storage_access_api/status.h"
#include "net/traffic_annotation/network_traffic_annotation.h"
#include "net/websockets/websocket_channel.h"
#include "net/websockets/websocket_event_interface.h"
#include "net/websockets/websocket_handshake_request_info.h"
#include "net/websockets/websocket_handshake_response_info.h"
#include "url/gurl.h"
#include "url/origin.h"
#include "net/tools/cronet/cronet_engine.h"

namespace {
constexpr net::NetworkTrafficAnnotationTag kTrafficAnnotation =
    net::DefineNetworkTrafficAnnotation("cronet_websocket", R"()");
}  // namespace

// Adapter class that implements WebSocketEventInterface and forwards events
// to the parent CronetWebSocket. This breaks the circular ownership:
// CronetWebSocket -> channel_ -> adapter (via unique_ptr)
// adapter -> CronetWebSocket* (raw pointer, non-owning)
class CronetWebSocketAdapter : public net::WebSocketEventInterface {
 public:
  explicit CronetWebSocketAdapter(CronetWebSocket* websocket)
      : websocket_(websocket) {}

  // WebSocketEventInterface overrides
  void OnCreateURLRequest(net::URLRequest* request) override {}

  int OnURLRequestConnected(
      net::URLRequest* request,
      const net::TransportInfo& info,
      net::CompletionOnceCallback callback) override {
    return net::OK;
  }

  void OnAddChannelResponse(
      std::unique_ptr<net::WebSocketHandshakeResponseInfo> response,
      const std::string& selected_subprotocol,
      const std::string& extensions) override {
    if (websocket_) {
      websocket_->OnConnected(selected_subprotocol);
      websocket_->StartReading();
    }
  }

  void OnDataFrame(bool fin,
                   WebSocketMessageType type,
                   base::span<const char> payload) override {
    LOG(ERROR) << "[cronet-ws-adapter] OnDataFrame: fin=" << fin
               << " type=" << static_cast<int>(type)
               << " payload_size=" << payload.size();
    if (!websocket_) return;

    // Type is 1 = text, 2 = binary, 0 = continuation
    if (type != 0) {
      // Start of a new message
      current_message_type_ = type;
      message_buffer_.clear();
    }

    message_buffer_.append(payload.data(), payload.size());

    if (fin) {
      // Complete message received
      int is_binary = (current_message_type_ == 2) ? 1 : 0;
      LOG(ERROR) << "[cronet-ws-adapter] delivering message: size="
                 << message_buffer_.size() << " is_binary=" << is_binary;
      websocket_->OnMessage(message_buffer_, is_binary);
      message_buffer_.clear();
    }

    if (websocket_) {
      websocket_->StartReading();
    }
  }

  bool HasPendingDataFrames() override { return false; }

  void OnSendDataFrameDone() override {}

  void OnClosingHandshake() override {}

  void OnDropChannel(bool was_clean,
                     uint16_t code,
                     const std::string& reason) override {
    if (websocket_) {
      websocket_->OnClosed(was_clean, code, reason);
    }
  }

  void OnFailChannel(const std::string& message,
                     int net_error,
                     std::optional<int> response_code) override {
    LOG(ERROR) << "[cronet-ws-adapter] OnFailChannel: net_error=" << net_error
               << " message=" << message;
    if (websocket_) {
      websocket_->OnError(net_error, message);
    }
  }

  void OnStartOpeningHandshake(
      std::unique_ptr<net::WebSocketHandshakeRequestInfo> request) override {}

  void OnSSLCertificateError(
      std::unique_ptr<net::WebSocketEventInterface::SSLErrorCallbacks>
          ssl_error_callbacks,
      const GURL& url,
      int net_error,
      const net::SSLInfo& ssl_info,
      bool fatal) override {
    if (websocket_ && websocket_->engine() &&
        websocket_->engine()->insecure_skip_verify() && !fatal) {
      ssl_error_callbacks->ContinueSSLRequest();
      return;
    }
    ssl_error_callbacks->CancelSSLRequest(net_error, &ssl_info);
  }

  int OnAuthRequired(
      const net::AuthChallengeInfo& auth_info,
      scoped_refptr<net::HttpResponseHeaders> response_headers,
      const net::IPEndPoint& host_port_pair,
      base::OnceCallback<void(const net::AuthCredentials*)> callback,
      std::optional<net::AuthCredentials>* credentials) override {
    // No auth support for WebSocket connections.
    return net::ERR_FAILED;
  }

  // Called when the parent CronetWebSocket is being destroyed
  void Detach() { websocket_ = nullptr; }

 private:
  CronetWebSocket* websocket_;  // Non-owning pointer
  std::string message_buffer_;
  int current_message_type_ = 0;
};

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
  if (!engine_ || !engine_->io_thread()) {
    return;
  }

  auto task_runner = engine_->io_thread()->task_runner();
  if (!task_runner) {
    return;
  }

  if (task_runner->RunsTasksInCurrentSequence()) {
    channel_.reset();
  } else {
    base::WaitableEvent done;
    task_runner->PostTask(
        FROM_HERE,
        base::BindOnce(&CronetWebSocket::DestroyOnIOThread,
                       base::Unretained(this), &done));
    done.Wait();
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
  // Create adapter that holds a non-owning pointer to this
  auto adapter = std::make_unique<CronetWebSocketAdapter>(this);
  CronetWebSocketAdapter* adapter_ptr = adapter.get();

  channel_ = std::make_unique<net::WebSocketChannel>(
      std::move(adapter),
      engine_->context());

  // Store adapter pointer for detachment on destruction
  adapter_ = adapter_ptr;

  GURL gurl(url);
  url::Origin origin = url::Origin::Create(gurl);
  net::IsolationInfo isolation_info =
      net::IsolationInfo::CreateTransient(std::nullopt);

  channel_->SendAddChannelRequest(
      gurl, subprotocols, origin,
      net::StorageAccessApiStatus::kNone, isolation_info, extra_headers_,
      kTrafficAnnotation);
}

void CronetWebSocket::SendOnIOThread(std::string data, int is_binary) {
  if (!channel_) {
    LOG(ERROR) << "[cronet-ws] SendOnIOThread: channel_ is null";
    return;
  }

  auto buffer = base::MakeRefCounted<net::IOBufferWithSize>(data.size());
  if (!data.empty()) {
    memcpy(buffer->data(), data.data(), data.size());
  }

  net::WebSocketEventInterface::WebSocketMessageType opcode =
      is_binary ? net::WebSocketEventInterface::WebSocketMessageType(2)
                : net::WebSocketEventInterface::WebSocketMessageType(1);

  LOG(ERROR) << "[cronet-ws] SendOnIOThread: sending " << data.size()
             << " bytes, opcode=" << static_cast<int>(opcode);
  auto result = channel_->SendFrame(true, opcode, std::move(buffer),
                                    data.size());
  LOG(ERROR) << "[cronet-ws] SendOnIOThread: SendFrame returned "
             << static_cast<int>(result);
}

void CronetWebSocket::CloseOnIOThread(uint16_t code,
                                       const std::string& reason) {
  if (channel_) {
    std::ignore = channel_->StartClosingHandshake(code, reason);
  }
}

void CronetWebSocket::DestroyOnIOThread(base::WaitableEvent* done) {
  // Detach adapter from this before destroying channel
  if (adapter_) {
    adapter_->Detach();
    adapter_ = nullptr;
  }
  channel_.reset();
  done->Signal();
}

// -- Event handlers called by adapter --

void CronetWebSocket::OnConnected(const std::string& selected_subprotocol) {
  LOG(ERROR) << "[cronet-ws] OnConnected: subprotocol="
             << selected_subprotocol << " channel_=" << (channel_ ? "ok" : "null");
  if (callbacks_.on_connected) {
    callbacks_.on_connected(this, selected_subprotocol.c_str(), user_data_);
  }
}

void CronetWebSocket::OnMessage(const std::string& data, int is_binary) {
  if (callbacks_.on_message) {
    callbacks_.on_message(this, data.data(), data.size(), is_binary, user_data_);
  }
}

void CronetWebSocket::OnClosed(bool was_clean, uint16_t code,
                               const std::string& reason) {
  if (callbacks_.on_closed) {
    callbacks_.on_closed(this, was_clean ? 1 : 0, code,
                        reason.empty() ? nullptr : reason.c_str(), user_data_);
  }
}

void CronetWebSocket::OnError(int net_error, const std::string& message) {
  LOG(ERROR) << "[cronet-ws] OnError: net_error=" << net_error
             << " message=" << message;
  if (callbacks_.on_error) {
    callbacks_.on_error(this, net_error,
                       message.empty() ? nullptr : message.c_str(),
                       user_data_);
  }
}

void CronetWebSocket::StartReading() {
  if (!engine_ || !engine_->io_thread()) {
    return;
  }
  engine_->io_thread()->task_runner()->PostTask(
      FROM_HERE, base::BindOnce(&CronetWebSocket::ReadFramesOnIOThread,
                                base::Unretained(this)));
}

void CronetWebSocket::ReadFramesOnIOThread() {
  if (!channel_) {
    return;
  }
  channel_->ReadFrames();
}
