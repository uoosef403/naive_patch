/*
 * cronet_request.cc -- HTTP request: URLRequest::Delegate implementation.
 */
#include "tools/cronet/cronet_request.h"

#include "base/functional/bind.h"
#include "base/task/single_thread_task_runner.h"
#include "net/base/elements_upload_data_stream.h"
#include "net/base/io_buffer.h"
#include "net/base/net_errors.h"
#include "net/base/upload_bytes_element_reader.h"
#include "net/http/http_response_headers.h"
#include "net/traffic_annotation/network_traffic_annotation.h"
#include "tools/cronet/cronet_engine.h"

namespace {
constexpr int kReadBufferSize = 65536;

constexpr net::NetworkTrafficAnnotationTag kTrafficAnnotation =
    net::DefineNetworkTrafficAnnotation("cronet_request", R"()");
}  // namespace

CronetRequest::CronetRequest(CronetEngine* engine,
                             const char* method,
                             const char* url,
                             const CronetRequestCallbacks* callbacks,
                             void* user_data)
    : engine_(engine),
      method_(method ? method : "GET"),
      url_(url ? url : ""),
      user_data_(user_data) {
  if (callbacks) {
    callbacks_ = *callbacks;
  } else {
    memset(&callbacks_, 0, sizeof(callbacks_));
  }
  read_buffer_ = base::MakeRefCounted<net::IOBufferWithSize>(kReadBufferSize);
}

CronetRequest::~CronetRequest() {
  // If url_request_ is still alive, cancel it on the IO thread.
  if (url_request_) {
    base::WaitableEvent done;
    engine_->io_thread()->task_runner()->PostTask(
        FROM_HERE,
        base::BindOnce(
            [](std::unique_ptr<net::URLRequest>* req,
               base::WaitableEvent* event) {
              req->reset();
              event->Signal();
            },
            &url_request_, &done));
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

void CronetRequest::StartOnIOThread() {
  GURL gurl(url_);
  url_request_ = engine_->context()->CreateRequest(gurl, net::DEFAULT_PRIORITY,
                                                   this, kTrafficAnnotation);
  url_request_->set_method(method_);

  if (!extra_headers_.IsEmpty()) {
    url_request_->SetExtraRequestHeaders(extra_headers_);
  }

  // Upload body
  if (!upload_data_.empty()) {
    auto reader = std::make_unique<net::UploadBytesElementReader>(
        upload_data_.data(), upload_data_.size());
    std::vector<std::unique_ptr<net::UploadElementReader>> readers;
    readers.push_back(std::move(reader));
    url_request_->set_upload(
        std::make_unique<net::ElementsUploadDataStream>(std::move(readers), 0));

    if (!upload_content_type_.empty()) {
      url_request_->SetExtraRequestHeaderByName(
          "Content-Type", upload_content_type_, true);
    }
  }

  url_request_->Start();
}

void CronetRequest::CancelOnIOThread() {
  if (url_request_) {
    url_request_->Cancel();
    OnRequestComplete(net::ERR_ABORTED);
  }
}

// -- URLRequest::Delegate overrides --

void CronetRequest::OnReceivedRedirect(
    net::URLRequest* request,
    const net::RedirectInfo& redirect_info,
    bool* defer_redirect) {
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

  // Cache the negotiated protocol
  negotiated_protocol_ = request->response_info().alpn_negotiated_protocol;

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
    const char* status_text = "";
    if (request->response_headers()) {
      status_text = request->response_headers()->GetStatusText().c_str();
    }
    int result = callbacks_.on_response_started(this, status_code_, status_text,
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
  if (callbacks_.on_completed) {
    callbacks_.on_completed(this, net_error, user_data_);
  }
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
