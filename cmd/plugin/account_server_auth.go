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
 if strings.ContainsRune(raw,rune(13)) || strings.ContainsRune(raw,rune(10)) || strings.ContainsRune(raw,rune(9)) {return false}}
 return true
}
func validAccountID(id string) bool {
 if len(id)<3||len(id)>128{return false}
 for _,r:=range id {
  if !(r>='a'&&r<='z'||r>='0'&&r<='9'||r=='-'||r=='_'){return false}
 }
 return true
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
 return managementJSON(200,map[string]any{"ok":true,"state":result,
  "message":map[string]string{
   "pending":"等待账号授权","failed":"账号授权失败，请重试","authorized":"账号已完成授权，请同步模型",
  }[result]})
}
