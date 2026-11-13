package main

// Server-side account authorization bridge. CLIProxyAPI ManagementAPI protects
// these routes, and the remote browser never calls the account runtime directly.
// Only provider-native device-code flows with externally reachable HTTPS
// verification URLs are enabled here. Loopback OAuth redirects are NOT usable
// by a remote browser and are explicitly not presented as supported.

import (
 "encoding/json"
 "errors"
 "fmt"
 "net"
 "net/http"
 "net/url"
 "strings"
)

var serverDeviceLoginProviders = map[string]bool{
 "qoder":true, "qodercn":true, "cline":true, "minimax":true,
}

type serverAccountRequest struct {
 Provider string `json:"provider"`
 AccountID string `json:"account_id"`
}
type serverRPCEnvelope struct {
 OK bool `json:"ok"`
 Value json.RawMessage `json:"value"`
 Error struct{
  Code string `json:"code"`
  Message string `json:"message"`
 } `json:"error"`
}
type remoteAccountStatus struct {
 ID string `json:"id"`
 Provider string `json:"provider"`
 Nickname string `json:"nickname"`
 Enabled bool `json:"enabled"`
 Refreshable bool `json:"refreshable"`
 ExpiresAt any `json:"expires_at,omitempty"`
}
func accountServerRPC(method string,payload any,callbackID string)(json.RawMessage,error) {
 cfg,_:=accountRoutesSnapshot()
 base,err:=safeGatewayURL(cfg.BaseURL)
 if err!=nil{return nil,errors.New("account service origin is invalid")}
 key,err:=gatewayKey()
 if err!=nil{return nil,errors.New("server account runtime is not authenticated")}
 wire,err:=json.Marshal(map[string]any{"method":method,"payload":payload})
 if err!=nil{return nil,err}
 if len(wire)>8192{return nil,errors.New("account RPC request too large")}
 var result httpResp
 err=hostCall("host.http.do",map[string]any{
  "host_callback_id":callbackID,"method":"POST",
  "url":base+"/api/management/channels/rpc",
  "headers":http.Header{"Authorization":{"Bearer "+key},
    "Content-Type":{"application/json"},"Accept":{"application/json"}},
  "body":wire,
 },&result)
 if err!=nil{return nil,errors.New("server account runtime is unreachable")}
 if result.StatusCode!=200{return nil,fmt.Errorf("account runtime rejected request (HTTP %d)",result.StatusCode)}
 if len(result.Body)>1024*1024{return nil,errors.New("oversize account response")}
 var response serverRPCEnvelope
 if err:=json.Unmarshal(result.Body,&response);err!=nil{return nil,errors.New("invalid account response")}
 if !response.OK{return nil,errors.New("account runtime operation failed")}
 return response.Value,nil
}
func safeRemoteLoginURL(raw string) bool {
 if len(raw)>4096{return false}
 u,err:=url.Parse(strings.TrimSpace(raw))
 if err!=nil||u.Scheme!="https"||u.Host==""||u.User!=nil||u.Fragment!=""{return false}
 host:=strings.ToLower(u.Hostname())
 if host=="localhost"||strings.HasSuffix(host,".localhost")||strings.HasSuffix(host,".local"){return false}
 if ip:=net.ParseIP(host);ip!=nil{return false}
 if strings.ContainsRune(raw,rune(13)) || strings.ContainsRune(raw,rune(10)) || strings.ContainsRune(raw,rune(9)) {return false}
 return true
}
func validAccountID(id string) bool {
 if len(id)<3||len(id)>128{return false}
 for _,r:=range id {
  if !(r>='a'&&r<='z'||r>='0'&&r<='9'||r=='-'||r=='_'){return false}
 }
 return true
}

var supportedAccountChannels = []string{
 "lobsterai", "qoder", "qodercn", "trae", "cline", "loomy",
 "raccoon", "minimax", "zcode", "gemini",
}

// Status is sourced from the sidecar account pool, not inferred from a
// possibly-stale public model catalog. "Registered" != "usable".
func serverChannelStatus(callbackID string) []byte {
 value, err := accountServerRPC("provider.status",
  map[string]any{"providers":supportedAccountChannels}, callbackID)
 if err!=nil{return managementError(502,"account_runtime_unavailable",err.Error())}
 var payload struct {
  Statuses map[string]struct{
   Accounts struct{
    Total int `json:"total"`
    Enabled int `json:"enabled"`
   } `json:"accounts"`
   Models struct{
    Total int `json:"total"`
    Disabled int `json:"disabled"`
   } `json:"models"`
  } `json:"statuses"`
 }
 if json.Unmarshal(value,&payload)!=nil{
  return managementError(502,"invalid_status","账号服务状态格式错误")
 }
 summary:=map[string]any{}
 for _,name:=range supportedAccountChannels {
  item,found:=payload.Statuses[name]
  if !found{continue}
  // Sanitize possibly malformed remote counters; do not invent "connected".
  if item.Accounts.Total<0||item.Accounts.Enabled<0||item.Accounts.Enabled>item.Accounts.Total||
    item.Models.Total<0||item.Models.Disabled<0||item.Models.Disabled>item.Models.Total {continue}
  summary[name]=map[string]any{
   "accounts_total":item.Accounts.Total,
   "accounts_enabled":item.Accounts.Enabled,
   "models_total":item.Models.Total,
   "models_disabled":item.Models.Disabled,
   "verified":false,
  }
 }
 return managementJSON(200,map[string]any{
  "ok":true,"statuses":summary,
  "note":"统计来自账号运行时的登记记录，并非实际登录、额度或推理成功证明",
 })
}

func serverAccountList(raw []byte,callbackID string) []byte {
 var req serverAccountRequest
 if len(raw)>2048||json.Unmarshal(raw,&req)!=nil{return managementError(400,"bad_request","请求无效")}
 if _,ok:=accountChannels[req.Provider];!ok{return managementError(400,"unknown_provider","账号渠道无效")}
 value,err:=accountServerRPC("account.list",map[string]any{"provider":req.Provider},callbackID)
 if err!=nil{return managementError(502,"account_runtime_unavailable",err.Error())}
 var data struct { Accounts []struct {
  ID string `json:"id"`
  Provider string `json:"provider"`
  Nickname string `json:"nickname"`
  Enabled bool `json:"enabled"`
  Refreshable bool `json:"refreshable"`
  ExpiresAt any `json:"expiresAt"`
 } `json:"accounts"` }
 if json.Unmarshal(value,&data)!=nil{return managementError(502,"bad_account_payload","无法解析账号列表")}
 out:=make([]remoteAccountStatus,0,len(data.Accounts))
 for _,a:=range data.Accounts {
  if a.ID=="" {continue}
  out=append(out,remoteAccountStatus{
   ID:a.ID,Provider:a.Provider,Nickname:a.Nickname,
   Enabled:a.Enabled,Refreshable:a.Refreshable,ExpiresAt:a.ExpiresAt,
  })
 }
 return managementJSON(200,map[string]any{"ok":true,"accounts":out})
}
func serverLoginStart(raw []byte,callbackID string) []byte {
 var req serverAccountRequest
 if len(raw)>2048||json.Unmarshal(raw,&req)!=nil{return managementError(400,"bad_request","请求无效")}
 if !serverDeviceLoginProviders[req.Provider] {
  return managementError(422,"needs_callback_adapter","该渠道采用本地回调或其它登录协议，尚未支持服务器远程授权")
 }
 value,err:=accountServerRPC("account.create",map[string]any{"provider":req.Provider},callbackID)
 if err!=nil{return managementError(502,"login_start_failed",err.Error())}
 var login struct {
  AccountID string `json:"accountId"`
  LoginURL string `json:"loginUrl"`
 }
 if json.Unmarshal(value,&login)!=nil||!validAccountID(login.AccountID)||!safeRemoteLoginURL(login.LoginURL) {
  return managementError(502,"login_not_remote","服务器账号服务未返回可远程打开的 HTTPS 授权链接")
 }
 // Never send backend credentials or the raw account RPC body to the browser.
 return managementJSON(200,map[string]any{"ok":true,
   "account_id":login.AccountID,"login_url":login.LoginURL,
   "message":"已发起授权，请在新窗口完成登录，然后返回检查状态"})
}
func serverLoginPoll(raw []byte,callbackID string) []byte {
 var req serverAccountRequest
 if len(raw)>2048||json.Unmarshal(raw,&req)!=nil||!validAccountID(req.AccountID) {
  return managementError(400,"bad_request","账号标识无效")
 }
 value,err:=accountServerRPC("login.poll",map[string]any{"accountId":req.AccountID},callbackID)
 if err!=nil{return managementError(502,"login_poll_failed",err.Error())}
 var state struct{
  Done bool `json:"done"`
  Success bool `json:"success"`
  Error string `json:"error"`
 }
 if json.Unmarshal(value,&state)!=nil{return managementError(502,"login_state_bad","无法读取授权状态")}
 result:="pending"
 if state.Done {
  result="failed"
  if state.Success{result="authorized"}
 }
 syncOK:=false
 syncedModels:=0
 reloadRequired:=false
 if result=="authorized" {
  // Authenticate with existing server-side Forward Key and update the account
  // model routing cache. Do not assume models appear in host /v1/models until
  // the host has re-run model.register.
  response:=connectAccountBridge([]byte("{}"),callbackID,false)
  var frame envelope
  if json.Unmarshal(response,&frame)==nil && frame.OK {
   var management struct{
    StatusCode int
    Body []byte
   }
   if json.Unmarshal(frame.Result,&management)==nil && management.StatusCode==200 {
    var details struct{
     OK bool `json:"ok"`
     ModelCount int `json:"model_count"`
    }
    if json.Unmarshal(management.Body,&details)==nil && details.OK {
     syncOK=true
     syncedModels=details.ModelCount
     reloadRequired=true
    }
   }
  }
 }
 message:="等待用户在授权页面确认"
 if result=="failed"{message="登录未成功或已过期，请重试"}
 if result=="authorized"{
  message="账号授权完成，已自动同步账号模型；宿主模型目录可能需要重载"
  if !syncOK {message="账号授权完成，但自动同步模型失败，请检查账号运行时连接并重试同步"}
 }
 return managementJSON(200,map[string]any{"ok":true,"state":result,
  "message":message,"model_sync_ok":syncOK,
  "synced_models":syncedModels,"reload_required":reloadRequired})
}
