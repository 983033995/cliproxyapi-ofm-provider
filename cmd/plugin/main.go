package main

/*
#include <stdint.h>
#include <stdlib.h>
typedef struct { void* ptr; size_t len; } cliproxy_buffer;
typedef int (*cliproxy_host_call_fn)(void*, const char*, const uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_host_free_fn)(void*, size_t);
typedef struct { uint32_t abi_version; void* host_ctx; cliproxy_host_call_fn call; cliproxy_host_free_fn free_buffer; } cliproxy_host_api;
typedef int (*cliproxy_plugin_call_fn)(char*, uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_plugin_free_fn)(void*, size_t);
typedef void (*cliproxy_plugin_shutdown_fn)(void);
typedef struct { uint32_t abi_version; cliproxy_plugin_call_fn call; cliproxy_plugin_free_fn free_buffer; cliproxy_plugin_shutdown_fn shutdown; } cliproxy_plugin_api;
extern int ofmPluginCall(char*, uint8_t*, size_t, cliproxy_buffer*);
extern void ofmPluginFree(void*, size_t);
extern void ofmPluginShutdown(void);
static const cliproxy_host_api* stored_host;
static void store_host(const cliproxy_host_api* host) { stored_host = host; }
static int invoke_host(const char* method, const uint8_t* req, size_t len, cliproxy_buffer* response) {
  if (!stored_host || !stored_host->call) return 1;
  return stored_host->call(stored_host->host_ctx, method, req, len, response);
}
static void release_host(void* ptr, size_t len) {
  if (ptr && stored_host && stored_host->free_buffer) stored_host->free_buffer(ptr, len);
}
*/
import "C"

import (
  "bytes"
  "encoding/json"
  "errors"
  "net/http"
  "strings"
  "sync"
  "unsafe"

  "github.com/983033995/cliproxyapi-ofm-provider/internal/provider"
)

const pluginName = "ofm-provider"
const maxRequestSize = 16 << 20

var runtime = struct {
  sync.RWMutex
  registry provider.Registry
}{registry: provider.Registry{Routes: map[string]provider.Route{}}}

type envelope struct {
  OK bool `json:"ok"`
  Result json.RawMessage `json:"result,omitempty"`
  Error *errDetail `json:"error,omitempty"`
}
type errDetail struct {
  Code string `json:"code"`
  Message string `json:"message"`
  HTTPStatus int `json:"http_status,omitempty"`
}
type rpcRequest struct {
  Model string
  Payload []byte
  StreamID string `json:"stream_id"`
  HostCallbackID string `json:"host_callback_id"`
}
type httpResp struct { StatusCode int; Headers http.Header; Body []byte }
type httpStreamResp struct {
  StatusCode int `json:"status_code"`
  Headers http.Header `json:"headers"`
  StreamID string `json:"stream_id"`
}
type httpChunk struct {
  Payload []byte `json:"payload"`
  Done bool `json:"done"`
  Error string `json:"error"`
}

func main() {}

//export cliproxy_plugin_init
func cliproxy_plugin_init(host *C.cliproxy_host_api, plugin *C.cliproxy_plugin_api) C.int {
  if host == nil || plugin == nil || host.abi_version != 1 { return 1 }
  C.store_host(host)
  plugin.abi_version = 1
  plugin.call = C.cliproxy_plugin_call_fn(C.ofmPluginCall)
  plugin.free_buffer = C.cliproxy_plugin_free_fn(C.ofmPluginFree)
  plugin.shutdown = C.cliproxy_plugin_shutdown_fn(C.ofmPluginShutdown)
  return 0
}

//export ofmPluginCall
func ofmPluginCall(method *C.char, request *C.uint8_t, length C.size_t, response *C.cliproxy_buffer) C.int {
  if response == nil { return 1 }
  response.ptr = nil
  response.len = 0
  if method == nil || uint64(length) > maxRequestSize || (length > 0 && request == nil) {
    writeResponse(response, fail("request_scoped", "invalid RPC request", 400))
    return 1
  }
  var raw []byte
  if length > 0 { raw = C.GoBytes(unsafe.Pointer(request), C.int(length)) }
  writeResponse(response, dispatch(C.GoString(method), raw))
  return 0
}

//export ofmPluginFree
func ofmPluginFree(ptr unsafe.Pointer, _ C.size_t) {
  if ptr != nil { C.free(ptr) }
}

//export ofmPluginShutdown
func ofmPluginShutdown() {}

func writeResponse(result *C.cliproxy_buffer, raw []byte) {
  if len(raw) != 0 {
    result.ptr = C.CBytes(raw)
    result.len = C.size_t(len(raw))
  }
}
func ok(v any) []byte {
  data, _ := json.Marshal(v)
  output, _ := json.Marshal(envelope{OK: true, Result: data})
  return output
}
func fail(code, message string, status int) []byte {
  output, _ := json.Marshal(envelope{OK: false, Error: &errDetail{
    Code: code, Message: message, HTTPStatus: status,
  }})
  return output
}
func load() error {
  cfg, err := readConfig()
  if err != nil { return err }
  loaded, err := provider.Validate(cfg)
  if err != nil { return err }
  runtime.Lock()
  runtime.registry = loaded
  runtime.Unlock()
  return loadAccounts()
}
func current() provider.Registry {
  runtime.RLock()
  defer runtime.RUnlock()
  return runtime.registry
}

func dispatch(method string, raw []byte) []byte {
  switch method {
  case "plugin.register", "plugin.reconfigure":
    if err := load(); err != nil {
      return fail("invalid_request", "provider registry could not be loaded", 400)
    }
    return ok(map[string]any{
      "schema_version": 10,
      "metadata": map[string]any{
        "Name": pluginName, "Version": "0.1.0-poc", "Author": "983033995", "ConfigFields": []any{},
        "GitHubRepository": "https://github.com/983033995/cliproxyapi-ofm-provider",
      },
      "capabilities": map[string]any{
        "model_registrar": true, "executor": true, "executor_model_scope": "static", "management_api": true, "auth_provider": true,
        "executor_input_formats": []string{"chat-completions"},
        "executor_output_formats": []string{"chat-completions"},
      },
    })
  case "auth.identifier":
    return nativeAuthIdentifierResponse()
  case "auth.parse":
    return parseHostAuth(raw)
  case "auth.refresh":
    return refreshHostAuth(raw)
  case "auth.login.start":
    return nativeStartLogin(raw)
  case "auth.login.poll":
    return nativePollLogin(raw)
  case "management.register":
    return registerManagement()
  case "management.handle":
    return handleManagement(raw)
  case "model.register":
    list := []map[string]any{}
    registry := current()
    for _, id := range registry.ModelIDs() {
      route, _ := registry.Resolve(id)
      info := map[string]any{
        "ID": id, "Object": "model", "OwnedBy": pluginName,
        "DisplayName": route.Model.Name, "SupportedGenerationMethods": []string{"chat"},
        "ContextLength": route.Model.ContextWindow, "UserDefined": true,
      }
      if route.Model.Vision { info["SupportedInputModalities"] = []string{"text", "image"} }
      list = append(list, info)
    }
    _, accountRoutes := accountRoutesSnapshot()
    for id, route := range accountRoutes {
      info := map[string]any{
        "ID": id, "Object": "model", "OwnedBy": pluginName,
        "DisplayName": route.DisplayName, "SupportedGenerationMethods": []string{"chat"},
        "ContextLength": route.ContextWindow, "UserDefined": true,
      }
      if route.Vision { info["SupportedInputModalities"] = []string{"text","image"} }
      list = append(list, info)
    }
    return ok(map[string]any{"Provider": pluginName, "Models": list})
  case "executor.identifier":
    return ok(map[string]any{"identifier": pluginName})
  case "executor.execute", "executor.execute_stream":
    var req rpcRequest
    if err := json.Unmarshal(raw, &req); err != nil {
      return fail("request_scoped", "invalid executor request", 400)
    }
    stream := method == "executor.execute_stream"
    if _, _, found := accountResolve(req.Model); found {
      return executeGateway(req,stream)
    }
    route, found := current().Resolve(req.Model)
    if !found { return fail("model_not_found", "unknown or disabled OFM model", 404) }
    key, err := credentialFor(route.Provider)
    if err != nil { return fail("invalid_api_key", "upstream credential is unavailable", 401) }
    streaming := method == "executor.execute_stream"
    payload, err := provider.Payload(req.Payload, route.Model.ID, streaming)
    if err != nil { return fail("request_scoped", "invalid chat-completions JSON", 400) }
    headers := http.Header{"Content-Type": {"application/json"}, "Accept": {"application/json"}}
    if streaming { headers.Set("Accept", "text/event-stream") }
    if key != "" { headers.Set("Authorization", "Bearer " + key) }
    for name, val := range route.Provider.Headers { headers.Set(name, val) }
    request := map[string]any{
      "host_callback_id": req.HostCallbackID, "method": "POST",
      "url": provider.Endpoint(route.Provider), "headers": headers, "body": payload,
    }
    if !streaming { return executeOnce(request) }
    if req.StreamID == "" { return fail("request_scoped", "downstream stream id is required", 400) }
    return executeStream(request, req.StreamID)
  case "executor.count_tokens":
    return fail("not_implemented", "token counting is not implemented", 501)
  default:
    return fail("not_implemented", "method is not supported", 501)
  }
}

func hostCall(method string, request any, dest any) error {
  body, err := json.Marshal(request)
  if err != nil { return err }
  cMethod := C.CString(method)
  defer C.free(unsafe.Pointer(cMethod))
  var data *C.uint8_t
  if len(body) != 0 {
    data = (*C.uint8_t)(C.CBytes(body))
    defer C.free(unsafe.Pointer(data))
  }
  var response C.cliproxy_buffer
  status := C.invoke_host(cMethod, data, C.size_t(len(body)), &response)
  if response.ptr == nil { return errors.New("host callback returned no payload") }
  raw := C.GoBytes(response.ptr, C.int(response.len))
  C.release_host(response.ptr, response.len)
  if status != 0 { return errors.New("host callback failed") }
  var output envelope
  if err := json.Unmarshal(raw, &output); err != nil { return err }
  if !output.OK { return errors.New("host callback rejected request") }
  if dest != nil { return json.Unmarshal(output.Result, dest) }
  return nil
}
func errorCode(status int) string {
  switch status {
  case 400: return "request_scoped"
  case 401: return "invalid_api_key"
  case 403: return "request_scoped"
  case 404: return "model_not_found"
  case 429: return "transient_rate_limit"
  default: return "transient_transport"
  }
}
func executeOnce(request any) []byte {
  var response httpResp
  if err := hostCall("host.http.do", request, &response); err != nil {
    return fail("transient_transport", "upstream transport unavailable", 502)
  }
  if response.StatusCode < 200 || response.StatusCode >= 300 {
    return fail(errorCode(response.StatusCode), "upstream returned a failure", response.StatusCode)
  }
  if !json.Valid(response.Body) {
    return fail("stream_truncated", "upstream did not return valid JSON", 502)
  }
  return ok(map[string]any{"Payload": response.Body, "Headers": response.Headers})
}
func executeStream(request any, downstreamID string) []byte {
  var response httpStreamResp
  if err := hostCall("host.http.do_stream", request, &response); err != nil {
    return fail("transient_transport", "upstream streaming request failed", 502)
  }
  if response.StreamID == "" {
    return fail("transient_transport", "host returned no upstream stream id", 502)
  }
  if response.StatusCode < 200 || response.StatusCode >= 300 {
    _ = hostCall("host.http.stream_close", map[string]any{"stream_id": response.StreamID}, nil)
    return fail(errorCode(response.StatusCode), "upstream refused streaming request", response.StatusCode)
  }
  if !strings.Contains(strings.ToLower(response.Headers.Get("Content-Type")), "text/event-stream") {
    _ = hostCall("host.http.stream_close", map[string]any{"stream_id": response.StreamID}, nil)
    return fail("stream_truncated", "upstream did not send SSE", 502)
  }
  go pumpStream(response.StreamID, downstreamID)
  return ok(map[string]any{"headers": http.Header{"Content-Type": {"text/event-stream"}}})
}
func pumpStream(upstreamID, downstreamID string) {
  defer func() {
    _ = hostCall("host.http.stream_close", map[string]any{"stream_id": upstreamID}, nil)
  }()
  closeStream := func(detail *errDetail) {
    message := map[string]any{"stream_id": downstreamID}
    if detail != nil { message["error_detail"] = detail }
    _ = hostCall("host.stream.close", message, nil)
  }
  var tail []byte
  terminal := false
  for {
    var chunk httpChunk
    err := hostCall("host.http.stream_read", map[string]any{"stream_id": upstreamID}, &chunk)
    if err != nil || chunk.Error != "" {
      closeStream(&errDetail{Code: "stream_truncated", Message: "upstream SSE interrupted", HTTPStatus: 502})
      return
    }
    if len(chunk.Payload) > 0 {
      tail = append(tail, chunk.Payload...)
      if bytes.Contains(tail, []byte("data: [DONE]")) { terminal = true }
      if len(tail) > 128 { tail = append([]byte(nil), tail[len(tail)-128:]...) }
      if err := hostCall("host.stream.emit", map[string]any{"stream_id": downstreamID, "payload": chunk.Payload}, nil); err != nil {
        closeStream(&errDetail{Code: "connection_lifecycle", Message: "downstream disconnected", HTTPStatus: 499})
        return
      }
    }
    if chunk.Done { break }
  }
  if !terminal {
    closeStream(&errDetail{Code: "stream_truncated", Message: "missing SSE terminal frame", HTTPStatus: 502})
    return
  }
  closeStream(nil)
}
