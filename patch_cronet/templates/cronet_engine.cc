/*
 * cronet_engine.cc -- Engine: IO thread, URLRequestContext construction.
 */
#include "tools/cronet/cronet_engine.h"

#include "base/at_exit.h"
#include "base/functional/bind.h"
#include "base/message_loop/message_pump_type.h"
#include "base/synchronization/waitable_event.h"
#include "base/task/single_thread_task_runner.h"
#include "net/cert/cert_verifier.h"
#include "net/dns/host_resolver.h"
#include "net/http/http_network_session.h"
#include "net/http/http_server_properties.h"
#include "net/proxy_resolution/configured_proxy_resolution_service.h"
#include "net/proxy_resolution/proxy_config_service_fixed.h"
#include "net/proxy_resolution/proxy_config_with_annotation.h"
#include "net/ssl/ssl_config_service_defaults.h"
#include "net/traffic_annotation/network_traffic_annotation.h"
#include "net/url_request/url_request_context.h"
#include "net/url_request/url_request_context_builder.h"
#include "url/gurl.h"

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

  // Cache
  if (params && params->cache_path && params->cache_path[0]) {
    net::URLRequestContextBuilder::HttpCacheParams cache_params;
    cache_params.type =
        net::URLRequestContextBuilder::HttpCacheParams::DISK_SIMPLE;
    cache_params.path = base::FilePath(params->cache_path);
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
