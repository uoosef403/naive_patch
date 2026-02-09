/*
 * cronet_request.h -- Internal request struct: URLRequest::Delegate impl.
 */
#ifndef NET_TOOLS_CRONET_CRONET_REQUEST_H_
#define NET_TOOLS_CRONET_CRONET_REQUEST_H_

#include <memory>
#include <string>
#include <vector>

#include "base/synchronization/waitable_event.h"
#include "base/timer/timer.h"
#include "net/base/io_buffer.h"
#include "net/base/load_timing_info.h"
#include "net/http/http_request_headers.h"
#include "net/ssl/ssl_info.h"
#include "net/url_request/url_request.h"
#include "net/url_request/url_request_context.h"
#include "net/tools/cronet/cronet_api.h"

struct CronetEngine;

struct CronetRequest : public net::URLRequest::Delegate {
  CronetRequest(CronetEngine* engine,
                const char* method,
                const char* url,
                const CronetRequestCallbacks* callbacks,
                void* user_data);
  ~CronetRequest() override;

  CronetRequest(const CronetRequest&) = delete;
  CronetRequest& operator=(const CronetRequest&) = delete;

  void AddHeader(const char* name, const char* value);
  void SetBody(const char* content_type, const char* data, size_t length);
  void SetParams(const CronetRequestParams* params);
  void Start();
  void Cancel();

  // Blocking wait for completion. Returns net_error.
  int Wait(int timeout_ms);

  int status_code() const { return status_code_; }
  const char* GetResponseHeader(const char* name);
  int GetResponseHeaderCount() const;
  const char* GetResponseHeaderName(int index) const;
  const char* GetResponseHeaderValue(int index) const;
  const char* GetNegotiatedProtocol() const;

  // Extended response info
  const char* GetUrl() const;
  int64_t GetContentLength() const;
  const char* GetHttpVersion() const;
  int GetTlsInfo(CronetTlsInfo* out) const;
  const char* GetTlsCertPem(int index) const;
  int GetTiming(CronetTimingInfo* out) const;

  // URLRequest::Delegate overrides
  void OnReceivedRedirect(net::URLRequest* request,
                          const net::RedirectInfo& redirect_info,
                          bool* defer_redirect) override;
  void OnResponseStarted(net::URLRequest* request, int net_error) override;
  void OnReadCompleted(net::URLRequest* request, int bytes_read) override;

 private:
  void StartOnIOThread();
  void CancelOnIOThread();
  void ReadData();
  void OnRequestComplete(int net_error);
  void OnTimeout();

  CronetEngine* engine_;
  std::string method_;
  std::string url_;
  CronetRequestCallbacks callbacks_;
  void* user_data_;

  net::HttpRequestHeaders extra_headers_;
  std::string upload_content_type_;
  std::string upload_data_;

  std::unique_ptr<net::URLRequest> url_request_;
  scoped_refptr<net::IOBufferWithSize> read_buffer_;

  int status_code_ = 0;
  std::string negotiated_protocol_;

  // Cached headers for enumeration
  std::vector<std::pair<std::string, std::string>> response_headers_;
  std::string header_lookup_buf_;  // temp buffer for GetResponseHeader

  // Per-request parameters
  int timeout_ms_ = 0;
  int connect_timeout_ms_ = 0;
  int max_redirects_ = -1;  // -1 = default (no limit from our side)
  int priority_ = 0;
  int disable_cache_ = 0;

  // Redirect tracking
  int redirect_count_ = 0;
  std::string final_url_;

  // HTTP version
  std::string http_version_string_;

  // Content length
  int64_t content_length_ = -1;

  // TLS info
  net::SSLInfo ssl_info_;
  bool has_ssl_info_ = false;
  std::vector<std::string> cert_pem_chain_;

  // Timing
  net::LoadTimingInfo load_timing_;
  bool has_timing_ = false;

  // Timeout timer (must be used on IO thread)
  std::unique_ptr<base::OneShotTimer> timeout_timer_;

  // Synchronous wait support
  base::WaitableEvent completion_event_;
  int completion_net_error_ = 0;
  bool completed_ = false;
};

#endif  // NET_TOOLS_CRONET_CRONET_REQUEST_H_
