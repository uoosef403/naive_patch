/*
 * cronet_request.h -- Internal request struct: URLRequest::Delegate impl.
 */
#ifndef NET_TOOLS_CRONET_CRONET_REQUEST_H_
#define NET_TOOLS_CRONET_CRONET_REQUEST_H_

#include <memory>
#include <string>
#include <vector>

#include "net/base/io_buffer.h"
#include "net/http/http_request_headers.h"
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
  void Start();
  void Cancel();

  int status_code() const { return status_code_; }
  const char* GetResponseHeader(const char* name);
  int GetResponseHeaderCount() const;
  const char* GetResponseHeaderName(int index) const;
  const char* GetResponseHeaderValue(int index) const;
  const char* GetNegotiatedProtocol() const;

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
};

#endif  // NET_TOOLS_CRONET_CRONET_REQUEST_H_
