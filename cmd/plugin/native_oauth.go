package main

// Native CLIProxyAPI v8 OAuth adapter for the account-channel runtime.
// The host owns OAuth session bookkeeping and auth file persistence.
// The OFM account runtime owns upstream OAuth PKCE, polling and tokens.
// Only public device-code verification pages work in a remote browser;
// localhost callback clients must never be advertised as remotely ready.

import (
 "crypto/rand"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "sync"
 "time"
)

type nativeAccountLogin struct {
 Channel string
 AccountID string
 Provider string
 ExpiresAt time.Time
}

var nativeLogins = struct {
 sync.Mutex
 ByState map[string]nativeAccountLogin
}{ByState:map[string]nativeAccountLogin{}}

var nativeAuthPortals=[]struct{
 Identifier string
 Channel string
 Label string
}{
 {"ofm-qoder","qoder","OFM · Qoder"},
 {"ofm-qodercn","qodercn","OFM · Qoder 中国版"},
 {"ofm-cline","cline","OFM · Cline"},
 {"ofm-minimax","minimax","OFM · MiniMax Code"},
}

func nativeChannel(provider string) (string,bool) {
 for _,p:=range nativeAuthPortals {
  if p.Identifier==provider {return p.Channel,true}
 }
 return "",false
}
func nativeAuthIdentifierResponse() []byte {
 entries:=make([]map[string]string,0,len(nativeAuthPortals))
 identifiers:=[]string{pluginName}
 for _,v:=range nativeAuthPortals {
  identifiers=append(identifiers,v.Identifier)
  entries=append(entries,map[string]string{"identifier":v.Identifier,"name":v.Label})
 }
 return ok(map[string]any{
  "identifier":pluginName,
  "identifiers":identifiers,
  "credential_providers":[]string{pluginName},
  "entries":entries,
 })
}
func nextNativeOAuthState() (string,error) {
 buf:=make([]byte,20)
 if _,err:=rand.Read(buf);err!=nil{return "",err}
 return "ofm-"+hex.EncodeToString(buf),nil
}
func rememberNativeOAuth(state string,p nativeAccountLogin) {
 nativeLogins.Lock()
 defer nativeLogins.Unlock()
 for k,v:=range nativeLogins.ByState {
  if time.Now().After(v.ExpiresAt){delete(nativeLogins.ByState,k)}
 }
 nativeLogins.ByState[state]=p
}
func lookupNativeOAuth(state,provider string)(nativeAccountLogin,bool) {
 nativeLogins.Lock()
 defer nativeLogins.Unlock()
 p,ok:=nativeLogins.ByState[state]
 if !ok||p.Provider!=provider||time.Now().After(p.ExpiresAt){
  if ok{delete(nativeLogins.ByState,state)}
  return nativeAccountLogin{},false
 }
 return p,true
}
func forgetNativeOAuth(state string) {
 nativeLogins.Lock()
 delete(nativeLogins.ByState,state)
 nativeLogins.Unlock()
}

type nativeAuthRequest struct {
 Provider string
 State string
 HostCallbackID string `json:"host_callback_id"`
}

func nativeStartLogin(raw []byte) []byte {
 var req nativeAuthRequest
 if len(raw)>16<<10 || json.Unmarshal(raw,&req)!=nil{
  return fail("invalid_request","invalid OAuth start request",400)
 }
 channel,allowed:=nativeChannel(req.Provider)
 if !allowed{return fail("not_implemented","this channel needs a remote callback adapter",422)}
 value,err:=accountServerRPC("account.create",map[string]any{"provider":channel},req.HostCallbackID)
 if err!=nil{return fail("transient_transport","account runtime login start failed",502)}
 var login struct{
  AccountID string `json:"accountId"`
  LoginURL string `json:"loginUrl"`
 }
 if json.Unmarshal(value,&login)!=nil||!validAccountID(login.AccountID)||!safeRemoteLoginURL(login.LoginURL) {
  return fail("request_scoped","runtime returned an invalid remote verification URL",502)
 }
 state,err:=nextNativeOAuthState()
 if err!=nil{return fail("transient_transport","cannot generate OAuth state",503)}
 expires:=time.Now().Add(9*time.Minute).UTC()
 rememberNativeOAuth(state,nativeAccountLogin{Channel:channel,
  Provider:req.Provider,AccountID:login.AccountID,ExpiresAt:expires})
 return ok(map[string]any{
  "Provider":req.Provider,"URL":login.LoginURL,
  "State":state,"ExpiresAt":expires,
  "Metadata":map[string]any{"channel":channel,"device_flow":true},
 })
}

func accountAuthMarker(login nativeAccountLogin) ([]byte,error) {
 if !validAccountID(login.AccountID){return nil,errors.New("invalid account identifier")}
 data:=map[string]string{
  "type":pluginName,
  "account_id":login.AccountID,
  "oauth_provider":login.Provider,
  "channel":login.Channel,
  "label":login.Channel+" · "+login.AccountID,
 }
 return json.Marshal(data)
}

func nativeAuthData(login nativeAccountLogin)(map[string]any,error) {
 raw,err:=accountAuthMarker(login)
 if err!=nil{return nil,err}
 return map[string]any{
  "Provider":pluginName,
  "ID":pluginName+"-"+login.AccountID,
  "FileName":pluginName+"-"+login.AccountID+".json",
  "Label":login.Channel+" · "+login.AccountID,
  "StorageJSON":raw,
  "Metadata":map[string]any{"oauth_provider":login.Provider},
  "Attributes":map[string]string{"oauth_provider":login.Provider},
 },nil
}

func nativePollLogin(raw []byte) []byte {
 var req nativeAuthRequest
 if len(raw)>16<<10||json.Unmarshal(raw,&req)!=nil{return fail("invalid_request","invalid OAuth poll request",400)}
 login,okFound:=lookupNativeOAuth(req.State,req.Provider)
 if !okFound{return ok(map[string]any{"Status":"error","Message":"授权状态已过期，请重新开始登录"})}
 value,err:=accountServerRPC("login.poll",map[string]any{"accountId":login.AccountID},req.HostCallbackID)
 if err!=nil{return fail("transient_transport","account runtime OAuth polling failed",502)}
 var result struct{
  Done bool `json:"done"`
  Success bool `json:"success"`
  Error string `json:"error"`
 }
 if json.Unmarshal(value,&result)!=nil{return fail("transient_transport","invalid OAuth poll response",502)}
 if !result.Done{return ok(map[string]any{"Status":"pending","Message":"等待用户完成设备码授权"})}
 forgetNativeOAuth(req.State)
 if !result.Success{
  return ok(map[string]any{"Status":"error","Message":"账号登录失败或取消，请重新尝试"})
 }
 // Refresh our model cache after the original runtime has persisted account tokens.
 // A separate host model-registration refresh may still be required.
 response:=connectAccountBridge([]byte("{}"),req.HostCallbackID,false)
 syncOK:=false
 var frame envelope
 if json.Unmarshal(response,&frame)==nil && frame.OK {
  var httpResult struct{ StatusCode int; Body []byte }
  if json.Unmarshal(frame.Result,&httpResult)==nil && httpResult.StatusCode==200 {
   var status struct{ OK bool `json:"ok"` }
   if json.Unmarshal(httpResult.Body,&status)==nil && status.OK{syncOK=true}
  }
 }
 auth,err:=nativeAuthData(login)
 if err!=nil{return fail("invalid_request","invalid completed account auth",400)}
 message:="账号已授权，模型缓存已更新；可能需重载 CLIProxyAPI"
 if !syncOK{message="账号已授权，但模型同步失败；请在管理页重新同步"}
 return ok(map[string]any{"Status":"success","Message":message,"Auth":auth})
}

// Compile-time documentation for the host's AuthLoginStart/Poll responses.
var _ = fmt.Sprintf
