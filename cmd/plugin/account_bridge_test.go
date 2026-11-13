package main

import (
    "encoding/json"
    "os"
    "path/filepath"
    "strings"
    "testing"
)

func TestAccountGatewayUsesLoopbackOnly(t *testing.T) {
    allowed := []string{"http://127.0.0.1:18900","http://localhost:18900","http://[::1]:18900/"}
    for _, raw := range allowed {
        got,err:=safeGatewayURL(raw)
        if err!=nil || got=="" { t.Fatalf("expected valid local gateway %s, err=%v",raw,err) }
    }
    blocked := []string{
        "https://example.com","http://example.com:18900",
        "http://192.168.1.55:18900","http://user:pass@127.0.0.1:18900",
        "http://127.0.0.1:18900/foo","http://127.0.0.1:18900?token=foo",
        "file:///tmp/key","http://127.0.0.1:18900#foo",
        "http://127.0.0.1","http://127.0.0.2:18900",
    }
    for _, raw := range blocked {
        if _,err:=safeGatewayURL(raw);err==nil {t.Errorf("allowed unsafe gateway %q",raw)}
    }
}
func TestOnlyAccountModelsEnterProviderRegistry(t *testing.T) {
    catalog:=gatewayModelCatalog{Data: []accountModel{
        {ID:"qoder/qfmodel",Name:"Qoder Flash",ContextWindow:128000},
        {ID:"cline/cline-free/deepseek-v4",Name:"Cline DeepSeek"},
        {ID:"trae/auto",Name:"TRAE"},
        {ID:"kilo/auto/free",Name:"Anonymous/Free"},
        {ID:"eac/demo"},
        {ID:"openrouter/openai/gpt"},
        {ID:"no-prefix"},
        {ID:"qoder/qfmodel",Name:"duplicate"},
    }}
    models,err:=filterChannelModels(catalog)
    if err!=nil {t.Fatal(err)}
    if len(models)!=3{t.Fatalf("want 3 account models got %d: %+v",len(models),models)}
    cfg:=accountGatewayConfig{Enabled:true,BaseURL:"http://127.0.0.1:18900",Models:models}
    routes:=modelRoutes(cfg)
    if len(routes)!=3{t.Fatalf("routes=%d",len(routes))}
    if _,ok:=routes["ofm-account/cline/cline-free/deepseek-v4"];!ok{t.Fatal("nested account model lost")}
    cfg.Enabled=false
    if len(modelRoutes(cfg))!=0{t.Fatal("disabled bridge leaked routes")}
}
func TestAccountGatewayStoresCredentialSeparateFromPublicConfig(t *testing.T) {
    temp:=t.TempDir()
    t.Setenv("OFM_PROVIDERS_FILE",filepath.Join(temp,"ofm-providers.json"))
    t.Setenv("OFM_SECRETS_FILE","")
    configMu.Lock()
    secrets,err:=readSecretsUnlocked()
    if err!=nil{configMu.Unlock();t.Fatal(err)}
    secrets[accountSecretID]="top-secret-local-service-key"
    if err:=writeSecretsUnlocked(secrets);err!=nil{configMu.Unlock();t.Fatal(err)}
    cfg:=accountGatewayConfig{Enabled:true,BaseURL:"http://127.0.0.1:18900",Models:[]accountModel{{ID:"qoder/qfmodel"}}}
    if err:=writeGatewayConfigUnlocked(cfg);err!=nil{configMu.Unlock();t.Fatal(err)}
    configMu.Unlock()
    path,_:=gatewayConfigPath()
    raw,err:=os.ReadFile(path)
    if err!=nil{t.Fatal(err)}
    if strings.Contains(string(raw),"top-secret"){t.Fatal("private key leaked to public gateway config")}
    var stored accountGatewayConfig
    if err:=json.Unmarshal(raw,&stored);err!=nil{t.Fatal(err)}
    if !stored.Enabled||len(stored.Models)!=1{t.Fatalf("wrong saved config: %+v",stored)}
    key,err:=gatewayKey()
    if err!=nil||key!="top-secret-local-service-key"{t.Fatal("gateway secret not recovered")}
}
func TestGatewayModelResponseParserDoesNotInventAccounts(t *testing.T) {
    payload:=[]byte(`{"object":"list","data":[{"id":"qoder/model","owned_by":"qoder"},{"id":"kilo/auto/free","owned_by":"kilo"},{"id":"opencode/unknown","owned_by":"opencode"}]}`)
    models,err:=parseGatewayModels(payload)
    if err!=nil{t.Fatal(err)}
    if len(models)!=1||models[0].ID!="qoder/model"{t.Fatalf("model filter mismatch: %+v",models)}
}
