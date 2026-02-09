/*
 * cronet_engine.h -- Internal engine struct: IO thread + URLRequestContext.
 */
#ifndef NET_TOOLS_CRONET_CRONET_ENGINE_H_
#define NET_TOOLS_CRONET_CRONET_ENGINE_H_

#include <memory>
#include <mutex>
#include <string>

#include "base/at_exit.h"
#include "base/memory/ref_counted.h"
#include "base/synchronization/waitable_event.h"
#include "base/threading/thread.h"
#include "net/url_request/url_request_context.h"
#include "net/tools/cronet/cronet_api.h"

struct CronetEngine {
  CronetEngine();
  ~CronetEngine();

  CronetEngine(const CronetEngine&) = delete;
  CronetEngine& operator=(const CronetEngine&) = delete;

  void Init(const CronetEngineParams* params);

  net::URLRequestContext* context() const { return context_.get(); }
  base::Thread* io_thread() { return &io_thread_; }

  bool insecure_skip_verify() const { return insecure_skip_verify_; }

  // Cookie management (blocks until operation completes on IO thread)
  void SetCookie(const char* url, const char* cookie_line);
  char* GetCookies(const char* url);
  void ClearCookies();

 private:
  void InitOnIOThread(const CronetEngineParams* params,
                      base::WaitableEvent* event);

  base::Thread io_thread_;
  std::unique_ptr<net::URLRequestContext> context_;
  bool insecure_skip_verify_ = false;

  // Prevents multiple engines from fighting over AtExitManager.
  static std::atomic<int> engine_count_;
  static std::unique_ptr<base::AtExitManager> at_exit_;
  static std::mutex at_exit_mutex_;
};

#endif  // NET_TOOLS_CRONET_CRONET_ENGINE_H_
