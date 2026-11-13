package main

import (
 "encoding/json"
 "regexp"
 "strings"
 "testing"
 "time"
)

func TestMultiAuthIdentifierSchemaAndEntries(t *testing.T) {
 tempConfig(t)
 var frame envelope
 if err:=json.Unmarshal(dispatch("plugin.register",nil),&frame);err!=nil||!frame.OK {
  t.Fatalf("register failed: %s %v",frame.Result,err)
 }
 var registration struct {
  SchemaVersion int `json:"schema_version"`
  Capabilities map[string]any `json:"capabilities"`
 }
 if err:=json.Unmarshal(frame.Result,&registration);err!=nil{t.Fatal(err)}
 if registration.SchemaVersion!=10{t.Fatalf("schema=%d; want 10",registration.SchemaVersion)}
 if registration.Capabilities["auth_provider"]!=true{t.Fatal("auth capability unavailable")}
 if err:=json.Unmarshal(dispatch("auth.identifier",nil),&frame);err!=nil||!frame.OK{
  t.Fatalf("auth.identifier failed: %v",err)
 }
 var identifiers struct {
  Identifier string `json:"identifier"`
  Identifiers []string `json:"identifiers"`
  CredentialProviders []string `json:"credential_providers"`
  Entries []struct {
   Identifier string `json:"identifier"`
   Name string `json:"name"`
  } `json:"entries"`
 }
 if err:=json.Unmarshal(frame.Result,&identifiers);err!=nil{t.Fatal(err)}
 if identifiers.Identifier!="ofm-qoder"||len(identifiers.Entries)!=4{t.Fatalf("identifier response: %#v",identifiers)}
 if len(identifiers.CredentialProviders)!=1||identifiers.CredentialProviders[0]!=pluginName{
  t.Fatalf("credential ownership missing: %#v",identifiers.CredentialProviders)
 }
 for _,entry:=range identifiers.Entries{
  if _,exists:=nativeChannel(entry.Identifier);!exists||entry.Name==""{t.Fatalf("invalid portal: %#v",entry)}
 }
}

func TestNativeOAuthStateIsOpaqueAndProviderScoped(t *testing.T) {
 a,err:=nextNativeOAuthState()
 if err!=nil{t.Fatal(err)}
 b,err:=nextNativeOAuthState()
 if err!=nil{t.Fatal(err)}
 if a==b||len(a)>128||!regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`).MatchString(a) {
  t.Fatalf("invalid random state %q",a)
 }
 p:=nativeAccountLogin{
  Provider:"ofm-qoder",Channel:"qoder",AccountID:"qoder-1234abcd",
  ExpiresAt:time.Now().Add(time.Minute),
 }
 rememberNativeOAuth(a,p)
 if _,ok:=lookupNativeOAuth(a,"ofm-cline");ok{t.Fatal("cross-provider state accepted")}
 value,ok:=lookupNativeOAuth(a,"ofm-qoder")
 if !ok||value.AccountID!=p.AccountID{t.Fatal("session not retained")}
 forgetNativeOAuth(a)
 if _,ok:=lookupNativeOAuth(a,"ofm-qoder");ok{t.Fatal("completed state still active")}
 p.ExpiresAt=time.Now().Add(-time.Second)
 rememberNativeOAuth(b,p)
 if _,ok:=lookupNativeOAuth(b,"ofm-qoder");ok{t.Fatal("expired state accepted")}
}

func TestNativeAuthFilesAreUniqueAndDoNotContainVendorSecrets(t *testing.T){
 for _,portal:=range nativeAuthPortals{
  login:=nativeAccountLogin{Provider:portal.Identifier,Channel:portal.Channel,
   AccountID:portal.Channel+"-1234abcd"}
  auth,err:=nativeAuthData(login)
  if err!=nil{t.Fatal(err)}
  if auth["Provider"]!=pluginName||auth["ID"]==pluginName{t.Fatalf("missing unique host auth id: %#v",auth)}
  data,ok:=auth["StorageJSON"].([]byte)
  if !ok {t.Fatal("StorageJSON must serialize as base64 bytes for host RPC")}
  if strings.Contains(string(data),"refresh_token")||strings.Contains(string(data),"access_token"){
   t.Fatal("third-party credential leaked to host auth marker")
  }
  parsed,handled:=authPayload(data,"")
  if !handled{t.Fatalf("refused owned auth marker for %s",portal.Identifier)}
  var rest map[string]any
  if err:=json.Unmarshal(parsed,&rest);err!=nil{t.Fatal(err)}
  if rest["ID"]!=auth["ID"]||rest["FileName"]!=auth["FileName"]{
   t.Fatalf("auth persisted identity drift: %#v",rest)
  }
  attrs:=rest["Attributes"].(map[string]any)
  if attrs["oauth_provider"]!=portal.Identifier{t.Fatalf("portal lost: %#v",attrs)}
 }
}

func TestInvalidNativePortalAndForgedAuthMarkerRejected(t *testing.T){
 if _,ok:=nativeChannel("ofm-trae");ok{t.Fatal("loopback callback channel wrongly registered")}
 invalid:=nativeAccountLogin{Provider:"ofm-qoder",Channel:"qoder",AccountID:"../secret"}
 if _,err:=nativeAuthData(invalid);err==nil{t.Fatal("unsafe ID accepted")}
 raw:=[]byte(`{"type":"ofm-provider","account_id":"qoder-12345678","oauth_provider":"untrusted"}`)
 if _,handled:=authPayload(raw,"");handled{t.Fatal("unknown OAuth provider accepted")}
}
