/*
 * cronet_request.cc -- HTTP request: URLRequest::Delegate implementation.
 */
#include "net/tools/cronet/cronet_request.h"

#include "base/containers/span.h"
#include "base/functional/bind.h"
#include "base/strings/string_number_conversions.h"
#include "base/strings/string_util.h"
#include "base/task/single_thread_task_runner.h"
#include "net/base/elements_upload_data_stream.h"
#include "net/base/io_buffer.h"
#include "net/base/net_errors.h"
#include "net/base/upload_bytes_element_reader.h"
#include "net/cert/x509_certificate.h"
#include "net/cert/x509_util.h"
#include "net/http/http_response_headers.h"
#include "net/http/http_version.h"
#include "net/ssl/ssl_connection_status_flags.h"
#include "net/traffic_annotation/network_traffic_annotation.h"
#include "net/tools/cronet/cronet_engine.h"

namespace {
constexpr int kReadBufferSize = 65536;

constexpr net::NetworkTrafficAnnotationTag kTrafficAnnotation =
    net::DefineNetworkTrafficAnnotation("cronet_request", R"()");

net::RequestPriority MapPriority(int priority) {
  switch (priority) {
    case CRONET_PRIORITY_IDLE:    return net::IDLE;
    case CRONET_PRIORITY_LOWEST:  return net::LOWEST;
    case CRONET_PRIORITY_LOW:     return net::LOW;
    case CRONET_PRIORITY_MEDIUM:  return net::MEDIUM;
    case CRONET_PRIORITY_HIGHEST: return net::HIGHEST;
    default:                      return net::DEFAULT_PRIORITY;
  }
}
}  // namespace

CronetRequest::CronetRequest(CronetEngine* engine,
                             const char* method,
                             const char* url,
                             const CronetRequestCallbacks* callbacks,
                             void* user_data)
    : engine_(engine),
      method_(method ? method : "GET"),
      url_(url ? url : ""),
      user_data_(user_data),
      completion_event_(base::WaitableEvent::ResetPolicy::MANUAL,
                        base::WaitableEvent::InitialState::NOT_SIGNALED) {
  if (callbacks) {
    callbacks_ = *callbacks;
  } else {
    memset(&callbacks_, 0, sizeof(callbacks_));
  }
  read_buffer_ = base::MakeRefCounted<net::IOBufferWithSize>(kReadBufferSize);
  final_url_ = url_;
}

CronetRequest::~CronetRequest() {
  // If url_request_ is still alive, cancel it on the IO thread.
  if (url_request_) {
    base::WaitableEvent done;
    engine_->io_thread()->task_runner()->PostTask(
        FROM_HERE,
        base::BindOnce(
            [](std::unique_ptr<net::URLRequest>* req,
               std::unique_ptr<base::OneShotTimer>* timer,
               base::WaitableEvent* event) {
              timer->reset();
              req->reset();
              event->Signal();
            },
            &url_request_, &timeout_timer_, &done));
    done.Wait();
  }
}

void CronetRequest::AddHeader(const char* name, const char* value) {
  if (name && value) {
    extra_headers_.SetHeader(name, value);
  }
}

void CronetRequest::SetBody(const char* content_type,
                            const char* data,
                            size_t length) {
  if (content_type) {
    upload_content_type_ = content_type;
  }
  if (data && length > 0) {
    upload_data_.assign(data, length);
  }
}

void CronetRequest::SetParams(const CronetRequestParams* params) {
  if (!params) return;
  timeout_ms_ = params->timeout_ms;
  connect_timeout_ms_ = params->connect_timeout_ms;
  max_redirects_ = params->max_redirects;
  priority_ = params->priority;
  disable_cache_ = params->disable_cache;
}

void CronetRequest::Start() {
  engine_->io_thread()->task_runner()->PostTask(
      FROM_HERE, base::BindOnce(&CronetRequest::StartOnIOThread,
                                base::Unretained(this)));
}

void CronetRequest::Cancel() {
  engine_->io_thread()->task_runner()->PostTask(
      FROM_HERE, base::BindOnce(&CronetRequest::CancelOnIOThread,
                                base::Unretained(this)));
}

int CronetRequest::Wait(int timeout_ms) {
  if (timeout_ms > 0) {
    bool signaled =
        completion_event_.TimedWait(base::Milliseconds(timeout_ms));
    if (!signaled) return net::ERR_TIMED_OUT;
  } else {
    completion_event_.Wait();
  }
  return completion_net_error_;
}

void CronetRequest::StartOnIOThread() {
  GURL gurl(url_);
  url_request_ = engine_->context()->CreateRequest(
      gurl, MapPriority(priority_), this, kTrafficAnnotation);
  url_request_->set_method(method_);

  if (!extra_headers_.IsEmpty()) {
    url_request_->SetExtraRequestHeaders(extra_headers_);
  }

  // Disable cache if requested
  if (disable_cache_) {
    url_request_->SetLoadFlags(url_request_->load_flags() |
                               net::LOAD_DISABLE_CACHE);
  }

  // Upload body
  if (!upload_data_.empty()) {
    auto reader = std::make_unique<net::UploadBytesElementReader>(
        base::as_byte_span(upload_data_));
    std::vector<std::unique_ptr<net::UploadElementReader>> readers;
    readers.push_back(std::move(reader));
    url_request_->set_upload(
        std::make_unique<net::ElementsUploadDataStream>(std::move(readers), 0));

    if (!upload_content_type_.empty()) {
      url_request_->SetExtraRequestHeaderByName(
          "Content-Type", upload_content_type_, true);
    }
  }

  // Start timeout timer
  if (timeout_ms_ > 0) {
    timeout_timer_ = std::make_unique<base::OneShotTimer>();
    timeout_timer_->Start(FROM_HERE, base::Milliseconds(timeout_ms_),
                          base::BindOnce(&CronetRequest::OnTimeout,
                                         base::Unretained(this)));
  }

  url_request_->Start();
}

void CronetRequest::CancelOnIOThread() {
  if (timeout_timer_) {
    timeout_timer_->Stop();
  }
  if (url_request_) {
    url_request_->Cancel();
    OnRequestComplete(net::ERR_ABORTED);
  }
}

void CronetRequest::OnTimeout() {
  if (url_request_) {
    url_request_->Cancel();
    OnRequestComplete(net::ERR_TIMED_OUT);
  }
}

// -- URLRequest::Delegate overrides --

void CronetRequest::OnReceivedRedirect(
    net::URLRequest* request,
    const net::RedirectInfo& redirect_info,
    bool* defer_redirect) {
  redirect_count_++;

  // Track URL through redirects
  final_url_ = redirect_info.new_url.spec();

  // Check max redirects
  if (max_redirects_ == 0) {
    // Don't follow any redirects -- stop here with the redirect status
    *defer_redirect = true;
    status_code_ = redirect_info.status_code;
    // Cache response headers from the redirect response
    if (request->response_headers()) {
      response_headers_.clear();
      size_t iter = 0;
      std::string name, value;
      while (request->response_headers()->EnumerateHeaderLines(&iter, &name,
                                                                &value)) {
        response_headers_.emplace_back(name, value);
      }
    }
    OnRequestComplete(net::OK);
    return;
  }
  if (max_redirects_ > 0 && redirect_count_ > max_redirects_) {
    request->Cancel();
    OnRequestComplete(net::ERR_TOO_MANY_REDIRECTS);
    return;
  }

  if (callbacks_.on_redirect) {
    int result = callbacks_.on_redirect(
        this, redirect_info.status_code,
        redirect_info.new_url.spec().c_str(), user_data_);
    if (result != 0) {
      request->Cancel();
      OnRequestComplete(net::ERR_ABORTED);
      return;
    }
  }
  // Allow redirect to proceed (default behavior).
}

void CronetRequest::OnResponseStarted(net::URLRequest* request,
                                       int net_error) {
  if (net_error != net::OK) {
    OnRequestComplete(net_error);
    return;
  }

  status_code_ = request->GetResponseCode();
  final_url_ = request->url().spec();

  // Cache the negotiated protocol
  negotiated_protocol_ = request->response_info().alpn_negotiated_protocol;

  // HTTP version
  if (request->response_headers()) {
    net::HttpVersion version = request->response_headers()->GetHttpVersion();
    if (negotiated_protocol_ == "h3") {
      http_version_string_ = "HTTP/3";
    } else if (version == net::HttpVersion(2, 0) ||
               negotiated_protocol_ == "h2") {
      http_version_string_ = "HTTP/2";
    } else if (version == net::HttpVersion(1, 1)) {
      http_version_string_ = "HTTP/1.1";
    } else if (version == net::HttpVersion(1, 0)) {
      http_version_string_ = "HTTP/1.0";
    } else {
      http_version_string_ = "HTTP/1.1";  // fallback
    }

    // Content-Length - parse from header directly for portability
    auto cl_value =
        request->response_headers()->GetNormalizedHeader("content-length");
    if (cl_value.has_value()) {
      int64_t parsed = -1;
      if (base::StringToInt64(*cl_value, &parsed)) {
        content_length_ = parsed;
      }
    }
  }

  // TLS info
  if (request->ssl_info().is_valid()) {
    ssl_info_ = request->ssl_info();
    has_ssl_info_ = true;

    // PEM-encode the certificate chain
    if (ssl_info_.unverified_cert) {
      std::string pem;
      if (net::X509Certificate::GetPEMEncoded(
              ssl_info_.unverified_cert->cert_buffer(), &pem)) {
        cert_pem_chain_.push_back(std::move(pem));
      }
      for (const auto& intermediate :
           ssl_info_.unverified_cert->intermediate_buffers()) {
        if (net::X509Certificate::GetPEMEncoded(intermediate.get(), &pem)) {
          cert_pem_chain_.push_back(std::move(pem));
        }
      }
    }
  }

  // Cache response headers for enumeration
  response_headers_.clear();
  if (request->response_headers()) {
    size_t iter = 0;
    std::string name, value;
    while (request->response_headers()->EnumerateHeaderLines(&iter, &name,
                                                              &value)) {
      response_headers_.emplace_back(name, value);
    }
  }

  // Notify caller
  if (callbacks_.on_response_started) {
    std::string status_text_str;
    if (request->response_headers()) {
      status_text_str = request->response_headers()->GetStatusText();
    }
    int result = callbacks_.on_response_started(this, status_code_,
                                                status_text_str.c_str(),
                                                user_data_);
    if (result != 0) {
      request->Cancel();
      OnRequestComplete(net::ERR_ABORTED);
      return;
    }
  }

  ReadData();
}

void CronetRequest::OnReadCompleted(net::URLRequest* request,
                                     int bytes_read) {
  if (bytes_read < 0) {
    OnRequestComplete(bytes_read);
    return;
  }

  if (bytes_read == 0) {
    OnRequestComplete(net::OK);
    return;
  }

  // Deliver data to caller
  if (callbacks_.on_data_received) {
    int result = callbacks_.on_data_received(this, read_buffer_->data(),
                                             bytes_read, user_data_);
    if (result != 0) {
      request->Cancel();
      OnRequestComplete(net::ERR_ABORTED);
      return;
    }
  }

  ReadData();
}

void CronetRequest::ReadData() {
  int bytes_read = url_request_->Read(read_buffer_.get(), kReadBufferSize);
  if (bytes_read == net::ERR_IO_PENDING) {
    return;  // OnReadCompleted will be called
  }
  OnReadCompleted(url_request_.get(), bytes_read);
}

void CronetRequest::OnRequestComplete(int net_error) {
  if (completed_) return;  // Guard against double completion
  completed_ = true;

  // Stop timeout timer
  if (timeout_timer_) {
    timeout_timer_->Stop();
  }

  // Capture timing info before signaling
  if (url_request_) {
    url_request_->GetLoadTimingInfo(&load_timing_);
    has_timing_ = !load_timing_.request_start.is_null();
  }

  completion_net_error_ = net_error;

  if (callbacks_.on_completed) {
    callbacks_.on_completed(this, net_error, user_data_);
  }

  // Signal any blocking waiter
  completion_event_.Signal();
}

// -- Response inspection --

const char* CronetRequest::GetResponseHeader(const char* name) {
  if (!name) return nullptr;
  for (const auto& pair : response_headers_) {
    if (base::EqualsCaseInsensitiveASCII(pair.first, name)) {
      header_lookup_buf_ = pair.second;
      return header_lookup_buf_.c_str();
    }
  }
  return nullptr;
}

int CronetRequest::GetResponseHeaderCount() const {
  return static_cast<int>(response_headers_.size());
}

const char* CronetRequest::GetResponseHeaderName(int index) const {
  if (index < 0 || index >= static_cast<int>(response_headers_.size()))
    return nullptr;
  return response_headers_[index].first.c_str();
}

const char* CronetRequest::GetResponseHeaderValue(int index) const {
  if (index < 0 || index >= static_cast<int>(response_headers_.size()))
    return nullptr;
  return response_headers_[index].second.c_str();
}

const char* CronetRequest::GetNegotiatedProtocol() const {
  return negotiated_protocol_.c_str();
}

// -- Extended response info --

const char* CronetRequest::GetUrl() const {
  return final_url_.c_str();
}

int64_t CronetRequest::GetContentLength() const {
  return content_length_;
}

const char* CronetRequest::GetHttpVersion() const {
  return http_version_string_.c_str();
}

int CronetRequest::GetTlsInfo(CronetTlsInfo* out) const {
  if (!has_ssl_info_ || !out) return -1;
  out->cipher_suite = net::SSLConnectionStatusToCipherSuite(
      ssl_info_.connection_status);
  out->tls_version = net::SSLConnectionStatusToVersion(
      ssl_info_.connection_status);
  out->negotiated_protocol = negotiated_protocol_.c_str();
  out->cert_chain_count = static_cast<int>(cert_pem_chain_.size());
  return 0;
}

const char* CronetRequest::GetTlsCertPem(int index) const {
  if (index < 0 || index >= static_cast<int>(cert_pem_chain_.size()))
    return nullptr;
  return cert_pem_chain_[index].c_str();
}

int CronetRequest::GetTiming(CronetTimingInfo* out) const {
  if (!has_timing_ || !out) return -1;
  auto base_time = load_timing_.request_start;
  auto to_ms = [&](base::TimeTicks t) -> int64_t {
    return t.is_null() ? -1 : (t - base_time).InMilliseconds();
  };
  out->dns_start_ms = to_ms(load_timing_.connect_timing.domain_lookup_start);
  out->dns_end_ms = to_ms(load_timing_.connect_timing.domain_lookup_end);
  out->connect_start_ms = to_ms(load_timing_.connect_timing.connect_start);
  out->connect_end_ms = to_ms(load_timing_.connect_timing.connect_end);
  out->ssl_start_ms = to_ms(load_timing_.connect_timing.ssl_start);
  out->ssl_end_ms = to_ms(load_timing_.connect_timing.ssl_end);
  out->send_start_ms = to_ms(load_timing_.send_start);
  out->send_end_ms = to_ms(load_timing_.send_end);
  out->first_byte_ms = to_ms(load_timing_.receive_headers_start);
  out->completion_ms = (base::TimeTicks::Now() - base_time).InMilliseconds();
  return 0;
}
