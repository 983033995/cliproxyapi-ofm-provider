package main

import (
 "encoding/json"
 "os"
 "path/filepath"
 "strings"
 "testing"

 "github.com/983033995/cliproxyapi-ofm-provider/internal/provider"
)

func tempConfig(t *testing.T) string {
 t.Helper()
 path:=filepath.Join(t.TempDir(),"config","providers.json")
 t.Setenv("OFM_PROVIDERS_FILE",path)
 t.Setenv("OFM_SECRETS_FILE","")
 return path
}
func readManagementJSON(t *testing.T, data []byte) (int,map[string]any) {
 t.Helper()
 var frame envelope
 if err:=json.Unmarshal(data,&frame);err!=nil {t.Fatal(err)}
 if !frame.OK{t.Fatalf("RPC error: %s",data)}
 var result struct{
  StatusCode int
  Body []byte
 }
 if err:=json.Unmarshal(frame.Result,&result);err!=nil {t.Fatal(err)}
 value:=map[string]any{}
 if err:=json.Unmarshal(result.Body,&value);err!=nil {t.Fatal(err)}
 return result.StatusCode,value
}
func TestEmbeddedCatalogAndDashboardDoNotExposeSecrets(t *testing.T){
 tempConfig(t)
 cfg,err:=readConfig()
 if err!=nil{t.Fatal(err)}
 if len(cfg.Providers)!=76{t.Fatalf("want 76 providers, got %d",len(cfg.Providers))}
 if len(dashboardHTML)==0||len(dashboardJS)==0||len(dashboardCSS)==0{t.Fatal("static assets missing")}
 if strings.Contains(string(dashboardHTML),"OFM_PROVIDERS_FILE") {t.Fatal("unexpected internal server path in resource")}
 count:=0
 for _,p:=range cfg.Providers{
  if p.Enabled{t.Fatal("catalog providers must default to disabled")}
  count+=len(p.Models)
 }
 if count!=507{t.Fatalf("want 507 models, got %d",count)}
}
func TestManagementSavesKeyWithoutEchoAndCanDisconnect(t *testing.T){
 configPath:=tempConfig(t)
 entry:=provider.Entry{ID:"demo",Name:"Demo API",BaseURL:"https://example.org/v1",
  AuthKind:"apiKey",Format:"openai-chat",KeyEnv:"DEMO_TEST_UNUSED",
  Models:[]provider.Model{{ID:"org/model",Name:"Model"}}}
 cfg:=provider.Config{Providers:[]provider.Entry{entry}}
 if err:=writeConfigUnlocked(cfg);err!=nil{t.Fatal(err)}
 enabled:=true
 req:=providerMutation{ID:"demo",Enabled:&enabled,APIKey:"private-test-token"}
 raw,_:=json.Marshal(req)
 status,result:=readManagementJSON(t,saveProviderResponse(raw))
 if status!=200||result["ok"]!=true {t.Fatalf("save failed: %#v",result)}
 key,err:=credentialFor(entry)
 if err!=nil||key!="private-test-token"{t.Fatalf("credential missing: %v",err)}
 secretPath,_:=secretsPath()
 st,err:=os.Stat(secretPath)
 if err!=nil{t.Fatal(err)}
 if st.Mode().Perm()!=0600{t.Fatalf("secrets file perms: %#o",st.Mode().Perm())}
 saved,_:=os.ReadFile(configPath)
 if strings.Contains(string(saved),"private-test-token"){t.Fatal("config contains secret")}
 list:=listProviderResponse()
 if strings.Contains(string(list),"private-test-token"){t.Fatal("API returned secret")}
 _,disconnectResult:=readManagementJSON(t,disconnectProviderResponse([]byte(`{"id":"demo"}`)))
 if disconnectResult["ok"]!=true{t.Fatal("disconnect failed")}
 p,_:=readConfig()
 if p.Providers[0].Enabled{t.Fatal("provider still enabled")}
 secrets,err:=readSecretsUnlocked()
 if err!=nil{t.Fatal(err)}
 if secrets["demo"]!=""{t.Fatal("key not deleted")}
}
func TestRejectUnsafeRepointAndUnsupportedAccount(t *testing.T){
 tempConfig(t)
 entry:=provider.Entry{ID:"remote",Name:"Remote",BaseURL:"https://old.example/v1",
  AuthKind:"apiKey",Format:"openai-chat",KeyEnv:"REMOTE_TEST_UNSET",
  Models:[]provider.Model{{ID:"m"}}}
 if err:=writeConfigUnlocked(provider.Config{Providers:[]provider.Entry{entry}});err!=nil{t.Fatal(err)}
 enabled:=true
 req:=providerMutation{ID:"remote",Enabled:&enabled,BaseURL:"https://new.example/v1",APIKey:"test"}
 raw,_:=json.Marshal(req)
 status,_:=readManagementJSON(t,saveProviderResponse(raw))
 if status!=409{t.Fatalf("changed endpoint without confirmation should fail, got %d",status)}
 unsupported:=provider.Entry{ID:"account",Name:"OAuth",AuthKind:"oauth",Format:"openai-chat",BaseURL:"https://x.example",Models:[]provider.Model{{ID:"a"}}}
 if err:=writeConfigUnlocked(provider.Config{Providers:[]provider.Entry{unsupported}});err!=nil{t.Fatal(err)}
 raw,_=json.Marshal(providerMutation{ID:"account",Enabled:&enabled,APIKey:"token"})
 status,_=readManagementJSON(t,saveProviderResponse(raw))
 if status!=400{t.Fatalf("unsupported auth should fail, got %d",status)}
}
func TestResourceRouteNeverReturnsProviderCatalog(t *testing.T){
 tempConfig(t)
 request:=map[string]any{"Method":"GET","Path":"/v0/resource/plugins/ofm-provider/dashboard"}
 raw,_:=json.Marshal(request)
 response:=handleManagement(raw)
 if strings.Contains(string(response),"private-test-token") {t.Fatal("resource leaked token")}
 if strings.Contains(string(response),`"providers"`) {t.Fatal("resource embedded API catalog")}
}
