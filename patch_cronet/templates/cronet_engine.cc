/*
 * cronet_engine.cc -- Engine: IO thread, URLRequestContext construction.
 */
#include "net/tools/cronet/cronet_engine.h"

#include <cstdlib>

#include "base/at_exit.h"
#include "base/files/file_path.h"
#include "base/functional/bind.h"
#include "base/message_loop/message_pump_type.h"
#include "base/synchronization/waitable_event.h"
#include "base/task/single_thread_task_runner.h"
#include "net/cert/cert_verifier.h"
#include "net/cert/x509_certificate.h"
#include "net/cookies/canonical_cookie.h"
#include "net/cookies/cookie_access_result.h"
#include "net/cookies/cookie_monster.h"
#include "net/cookies/cookie_options.h"
#include "net/cookies/cookie_store.h"
#include "net/dns/host_resolver.h"
#include "net/http/http_network_session.h"
#include "net/http/http_server_properties.h"
#include "net/proxy_resolution/configured_proxy_resolution_service.h"
#include "net/proxy_resolution/proxy_config_service_fixed.h"
#include "net/proxy_resolution/proxy_config_with_annotation.h"
#include "net/ssl/ssl_config_service.h"
#include "net/traffic_annotation/network_traffic_annotation.h"
#include "net/url_request/url_request_context.h"
#include "net/url_request/url_request_context_builder.h"
#include "url/gurl.h"

namespace {

// A CertVerifier that accepts all certificates unconditionally.
// Used for InsecureSkipVerify mode.
class AcceptAllCertVerifier : public net::CertVerifier {
 public:
  AcceptAllCertVerifier() = default;
  ~AcceptAllCertVerifier() override = default;

  int Verify(const net::CertVerifier::RequestParams& params,
             net::CertVerifyResult* verify_result,
             net::CompletionOnceCallback callback,
             std::unique_ptr<Request>* out_req,
             const net::NetLogWithSource& net_log) override {
    verify_result->verified_cert = params.certificate();
    verify_result->cert_status = 0;  // No errors
    return net::OK;
  }
  void SetConfig(const Config& config) override {}
  void AddObserver(Observer* observer) override {}
  void RemoveObserver(Observer* observer) override {}
  void Verify2QwacBinding(
      const std::string& binding,
      const std::string& hostname,
      const scoped_refptr<net::X509Certificate>& tls_cert,
      base::OnceCallback<void(const scoped_refptr<net::X509Certificate>&)> callback,
      const net::NetLogWithSource& net_log) override {
    // No QWAC verification needed - invoke callback with nullptr
    std::move(callback).Run(nullptr);
  }
};

// A simple SSLConfigService that returns a fixed SSLContextConfig.
class StaticSSLConfigService : public net::SSLConfigService {
 public:
  explicit StaticSSLConfigService(const net::SSLContextConfig& config)
      : config_(config) {}

  net::SSLContextConfig GetSSLContextConfig() override { return config_; }

  bool CanShareConnectionWithClientCerts(
      std::string_view hostname) const override {
    return false;
  }

 private:
  net::SSLContextConfig config_;
};

}  // namespace

// Static members
std::atomic<int> CronetEngine::engine_count_{0};
std::unique_ptr<base::AtExitManager> CronetEngine::at_exit_;
std::mutex CronetEngine::at_exit_mutex_;

CronetEngine::CronetEngine() : io_thread_("CronetIO") {
  std::lock_guard<std::mutex> lock(at_exit_mutex_);
  if (engine_count_.fetch_add(1) == 0) {
    at_exit_ = std::make_unique<base::AtExitManager>();
  }
}

CronetEngine::~CronetEngine() {
  // Destroy the context on the IO thread.
  if (io_thread_.IsRunning()) {
    base::WaitableEvent done;
    io_thread_.task_runner()->PostTask(
        FROM_HERE,
        base::BindOnce(
            [](std::unique_ptr<net::URLRequestContext>* ctx,
               base::WaitableEvent* event) {
              ctx->reset();
              event->Signal();
            },
            &context_, &done));
    done.Wait();
    io_thread_.Stop();
  }

  std::lock_guard<std::mutex> lock(at_exit_mutex_);
  if (engine_count_.fetch_sub(1) == 1) {
    at_exit_.reset();
  }
}

void CronetEngine::Init(const CronetEngineParams* params) {
  base::Thread::Options options;
  options.message_pump_type = base::MessagePumpType::IO;
  io_thread_.StartWithOptions(std::move(options));

  base::WaitableEvent init_done;
  io_thread_.task_runner()->PostTask(
      FROM_HERE, base::BindOnce(&CronetEngine::InitOnIOThread,
                                base::Unretained(this), params, &init_done));
  init_done.Wait();
}

void CronetEngine::InitOnIOThread(const CronetEngineParams* params,
                                  base::WaitableEvent* event) {
  net::URLRequestContextBuilder builder;

  // User agent
  if (params && params->user_agent && params->user_agent[0]) {
    builder.set_user_agent(params->user_agent);
  }

  // Proxy configuration
  if (params && params->proxy_url && params->proxy_url[0]) {
    net::ProxyConfig proxy_config;
    proxy_config.proxy_rules().ParseFromString(params->proxy_url);
    auto proxy_config_service =
        std::make_unique<net::ProxyConfigServiceFixed>(
            net::ProxyConfigWithAnnotation(
                proxy_config, net::DefineNetworkTrafficAnnotation(
                                  "cronet_proxy", R"()")));
    builder.set_proxy_config_service(std::move(proxy_config_service));
  } else {
    builder.set_proxy_config_service(
        std::make_unique<net::ProxyConfigServiceFixed>(
            net::ProxyConfigWithAnnotation::CreateDirect()));
  }

  // HTTP network session params
  net::HttpNetworkSessionParams session_params;
  if (params) {
    session_params.enable_http2 = params->enable_http2;
    session_params.enable_quic = params->enable_quic;
  } else {
    session_params.enable_http2 = true;
    session_params.enable_quic = true;
  }
  builder.set_http_network_session_params(session_params);

  // TLS: InsecureSkipVerify
  if (params && params->insecure_skip_verify) {
    insecure_skip_verify_ = true;
    builder.SetCertVerifier(std::make_unique<AcceptAllCertVerifier>());
  }

  // TLS: min/max version
  if (params && (params->tls_min_version || params->tls_max_version)) {
    net::SSLContextConfig ssl_ctx_config;
    if (params->tls_min_version) {
      ssl_ctx_config.version_min =
          static_cast<uint16_t>(params->tls_min_version);
    }
    if (params->tls_max_version) {
      ssl_ctx_config.version_max =
          static_cast<uint16_t>(params->tls_max_version);
    }
    builder.set_ssl_config_service(
        std::make_unique<StaticSSLConfigService>(ssl_ctx_config));
  }

  // Cookies
  if (params && params->enable_cookies) {
    builder.SetCookieStore(
        std::make_unique<net::CookieMonster>(nullptr, nullptr));
  }

  // Cache
  if (params && params->cache_path && params->cache_path[0]) {
    net::URLRequestContextBuilder::HttpCacheParams cache_params;
    cache_params.type =
        net::URLRequestContextBuilder::HttpCacheParams::DISK_SIMPLE;
    cache_params.path = base::FilePath::FromUTF8Unsafe(params->cache_path);
    if (params->cache_max_bytes > 0) {
      cache_params.max_size = params->cache_max_bytes;
    }
    builder.EnableHttpCache(cache_params);
  } else {
    builder.DisableHttpCache();
  }

  context_ = builder.Build();
  event->Signal();
}

// -- Cookie management --

void CronetEngine::SetCookie(const char* url, const char* cookie_line) {
  if (!url || !cookie_line || !context_) return;

  GURL gurl(url);
  if (!gurl.is_valid()) return;

  base::WaitableEvent done;
  io_thread_.task_runner()->PostTask(
      FROM_HERE,
      base::BindOnce(
          [](net::URLRequestContext* ctx, const GURL& gurl,
             const std::string& cookie_line, base::WaitableEvent* event) {
            auto cookie = net::CanonicalCookie::CreateForTesting(
                gurl, cookie_line, base::Time::Now());
            if (!cookie) {
              event->Signal();
              return;
            }
            ctx->cookie_store()->SetCanonicalCookieAsync(
                std::move(cookie), gurl,
                net::CookieOptions::MakeAllInclusive(),
                base::BindOnce(
                    [](base::WaitableEvent* e,
                       net::CookieAccessResult) { e->Signal(); },
                    event));
          },
          context_.get(), gurl, std::string(cookie_line), &done));
  done.Wait();
}

char* CronetEngine::GetCookies(const char* url) {
  if (!url || !context_) return nullptr;

  GURL gurl(url);
  if (!gurl.is_valid()) return nullptr;

  std::string result;
  base::WaitableEvent done;
  io_thread_.task_runner()->PostTask(
      FROM_HERE,
      base::BindOnce(
          [](net::URLRequestContext* ctx, const GURL& gurl,
             std::string* result, base::WaitableEvent* event) {
            ctx->cookie_store()->GetCookieListWithOptionsAsync(
                gurl, net::CookieOptions::MakeAllInclusive(),
                net::CookiePartitionKeyCollection(),
                base::BindOnce(
                    [](std::string* out, base::WaitableEvent* e,
                       const net::CookieAccessResultList& cookies,
                       const net::CookieAccessResultList&) {
                      for (size_t i = 0; i < cookies.size(); i++) {
                        if (i > 0) out->append("; ");
                        out->append(cookies[i].cookie.Name());
                        out->append("=");
                        out->append(cookies[i].cookie.Value());
                      }
                      e->Signal();
                    },
                    result, event));
          },
          context_.get(), gurl, &result, &done));
  done.Wait();

  if (result.empty()) return nullptr;

  char* copy = static_cast<char*>(malloc(result.size() + 1));
  memcpy(copy, result.c_str(), result.size() + 1);
  return copy;
}

void CronetEngine::ClearCookies() {
  if (!context_) return;

  base::WaitableEvent done;
  io_thread_.task_runner()->PostTask(
      FROM_HERE,
      base::BindOnce(
          [](net::URLRequestContext* ctx, base::WaitableEvent* event) {
            ctx->cookie_store()->DeleteAllAsync(
                base::BindOnce(
                    [](base::WaitableEvent* e, uint32_t) { e->Signal(); },
                    event));
          },
          context_.get(), &done));
  done.Wait();
}
