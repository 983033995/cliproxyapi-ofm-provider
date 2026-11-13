package main

import (
 "encoding/json"
 "fmt"
 "net/http"
 "os"
 "strings"

 "github.com/983033995/cliproxyapi-ofm-provider/internal/provider"
)
type accountBridgeMutation struct {
 BaseURL string `json:"base_url"`
 ForwardKey string `json:"forward_key"`
}
func accountBridgeStatus() []byte {
 configMu.Lock()
 defer configMu.Unlock()
 cfg, err:=readGatewayConfigUnlocked()
 if err!=nil{return managementError(500,"config_error","账号桥接配置读取失败")}
 secrets,err:=readSecretsUnlocked()
 if err!=nil{return managementError(500,"credential_error","账号桥接凭据不可读取")}
 _,routes:=accountRoutesSnapshot()
 list:=make([]accountModel,0,len(cfg.Models))
 list=append(list,cfg.Models...)
 return managementJSON(200,map[string]any{
  "ok":true,"enabled":cfg.Enabled,"base_url":cfg.BaseURL,
  "credential_present":accountKeyUnlocked(secrets)!="" || os.Getenv("OFM_ACCOUNT_FORWARD_KEY")!="",
  "connected_models":len(routes),"models":list,
  "source":"Ebony-Vinyl/dsh-our-free-model standalone service",
  "note":"账号授权和签到在原服务本地管理页完成。此处只同步已登录账号的模型；模型变更需重载 CLIProxyAPI。",
 })
}
func resolveGatewayKey(provided,stored string) string {
 if strings.TrimSpace(provided)!="" {return strings.TrimSpace(provided)}
 if strings.TrimSpace(stored)!="" {return strings.TrimSpace(stored)}
 return strings.TrimSpace(os.Getenv("OFM_ACCOUNT_FORWARD_KEY"))
}
func parseGatewayModels(body []byte) ([]accountModel,error) {
 var catalog gatewayModelCatalog
 if err:=json.Unmarshal(body,&catalog);err!=nil{return nil,fmt.Errorf("invalid model catalog: %w",err)}
 return filterChannelModels(catalog)
}
func connectAccountBridge(raw []byte, callbackID string, testOnly bool) []byte {
 if len(raw)>64<<10{return managementError(413,"too_large","请求内容过大")}
 var req accountBridgeMutation
 if err:=json.Unmarshal(raw,&req);err!=nil{return managementError(400,"bad_request","JSON 请求格式错误")}
 if len(req.ForwardKey)>8192 || strings.ContainsAny(req.ForwardKey,"\r\n"){
  return managementError(400,"bad_credential","本地服务 Key 格式错误")
 }
 // A caller cannot repoint stored credentials to a new address without
 // explicitly giving a new key. This avoids leaking it to a changed endpoint.
 configMu.Lock()
 old,err:=readGatewayConfigUnlocked()
 if err!=nil{configMu.Unlock();return managementError(500,"config_error","读取旧配置失败")}
 baseURL:=strings.TrimSpace(req.BaseURL)
 if baseURL=="" {baseURL=old.BaseURL}
 safeURL,err:=safeGatewayURL(baseURL)
 if err!=nil{configMu.Unlock();return managementError(400,"invalid_address",err.Error())}
 secrets,err:=readSecretsUnlocked()
 if err!=nil{configMu.Unlock();return managementError(500,"credential_error","读取密钥失败")}
 savedKey:=accountKeyUnlocked(secrets)
 if old.BaseURL!="" && old.BaseURL!=safeURL {
  if req.ForwardKey=="" {
   configMu.Unlock()
   return managementError(409,"confirm_server_repoint","服务器内部账号服务地址改变时必须重新提供 Forward Key，避免泄露已有密钥")
  }
  savedKey=""
 }
 key:=resolveGatewayKey(req.ForwardKey,savedKey)
 configMu.Unlock()
 if key=="" {return managementError(400,"missing_credential","请输入独立服务 API 接入页提供的 Forward Key")}
 var response httpResp
 err=hostCall("host.http.do",map[string]any{
  "host_callback_id":callbackID,"method":"GET","url":safeURL+"/v1/models",
  "headers":http.Header{"Authorization":{"Bearer "+key},"Accept":{"application/json"}},
 },&response)
 if err!=nil {
  return managementError(502,"network_unavailable","无法连接独立账号服务，请确认它在同一宿主上运行并检查端口")
 }
 if response.StatusCode!=200 {
  return managementError(400,"gateway_error",fmt.Sprintf("独立服务返回 HTTP %d。请确认 Forward Key 有效",response.StatusCode))
 }
 if len(response.Body)>4<<20{return managementError(502,"oversize","模型清单超出限制")}
 models,err:=parseGatewayModels(response.Body)
 if err!=nil{return managementError(502,"invalid_catalog","账号服务返回的模型清单无效")}
 if testOnly {
  return managementJSON(200,map[string]any{"ok":true,"model_count":len(models),
   "message":"已验证账号服务 API；生成调用仍需端到端验收"})
 }
 configMu.Lock()
 defer configMu.Unlock()
 // Re-check current endpoint while locked. Do not re-use a key with a changed URL.
 updated,err:=readGatewayConfigUnlocked()
 if err!=nil{return managementError(500,"config_error","读取配置失败")}
 if updated.BaseURL!=old.BaseURL {return managementError(409,"changed","地址已被其它管理会话修改，请重新连接")}
 updated.BaseURL=safeURL
 updated.Enabled=true
 updated.Models=models
 if req.ForwardKey!="" {secrets[accountSecretID]=key}
 if req.ForwardKey=="" && savedKey=="" && os.Getenv("OFM_ACCOUNT_FORWARD_KEY")=="" {
  return managementError(400,"missing_credential","密钥不可用")
 }
 if err:=writeSecretsUnlocked(secrets);err!=nil{
  return managementError(500,"save_error","无法将账号服务 Key 写入私有文件")
 }
 if err:=writeGatewayConfigUnlocked(updated);err!=nil{
  return managementError(500,"save_error","无法保存模型清单")
 }
 replaceAccounts(updated)
 // Keep host auth marker up to date; does not copy upstream credentials.
 // Host callback runs within the authenticated management request scope.
 var saved struct{ Name string; Path string }
 authErr:=hostCall("host.auth.save",map[string]any{
  "Name":"ofm-provider.json",
  "JSON":json.RawMessage(`{"type":"ofm-provider","label":"OFM Account Channel Bridge"}`),
 },&saved)
 return managementJSON(200,map[string]any{
  "ok":true,"model_count":len(models),"host_auth_ready":authErr==nil,
  "reload_required":true,
  "message":"账号服务连接成功；已同步账号渠道模型。若新增模型未出现，请重载 CLIProxyAPI。",
 })
}
func disconnectAccountBridge() []byte {
 configMu.Lock()
 defer configMu.Unlock()
 cfg,err:=readGatewayConfigUnlocked()
 if err!=nil{return managementError(500,"config_error","读取配置失败")}
 secrets,err:=readSecretsUnlocked()
 if err!=nil{return managementError(500,"credential_error","读取凭据失败")}
 cfg.Enabled=false
 cfg.Models=[]accountModel{}
 delete(secrets,accountSecretID)
 if err:=writeSecretsUnlocked(secrets);err!=nil{return managementError(500,"save_error","凭据更新失败")}
 if err:=writeGatewayConfigUnlocked(cfg);err!=nil{return managementError(500,"save_error","配置更新失败")}
 replaceAccounts(cfg)
 return managementJSON(200,map[string]any{"ok":true,"reload_required":true,
  "message":"已在插件中断开账号服务。原服务中的已登录账号未注销；可到其控制台单独退出。"})
}
func executeGateway(req rpcRequest, stream bool) []byte {
 cfg,route,found:=accountResolve(req.Model)
 if !found{return fail("model_not_found","unknown account channel model",404)}
 base,err:=safeGatewayURL(cfg.BaseURL)
 if err!=nil{return fail("request_scoped","invalid account gateway address",400)}
 configMu.Lock()
 secrets,err:=readSecretsUnlocked()
 var key string
 if err==nil{key=resolveGatewayKey("",accountKeyUnlocked(secrets))}
 configMu.Unlock()
 if err!=nil || key=="" {return fail("invalid_api_key","account gateway key unavailable",401)}
 payload,err:=provider.Payload(req.Payload,route.UpstreamModel,stream)
 if err!=nil{return fail("request_scoped","invalid chat-completions request",400)}
 headers:=http.Header{"Authorization":{"Bearer "+key},"Content-Type":{"application/json"},"Accept":{"application/json"}}
 if stream{headers.Set("Accept","text/event-stream")}
 upstream:=map[string]any{
  "host_callback_id":req.HostCallbackID,"method":"POST",
  "url":base+"/v1/chat/completions","headers":headers,"body":payload,
 }
 if !stream{return executeOnce(upstream)}
 if req.StreamID==""{return fail("request_scoped","missing downstream stream id",400)}
 return executeStream(upstream,req.StreamID)
}
