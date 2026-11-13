package main

import (
 "encoding/json"
 "strings"
 "testing"
)

func TestAuthMarkerParserAcceptsOnlyItsOwnType(t *testing.T) {
 request,_:=json.Marshal(authFileRequest{
  Provider: pluginName,FileName:"ofm-provider.json",
  RawJSON: []byte(`{"type":"ofm-provider","label":"Owner's providers"}`),
 })
 resp:=parseHostAuth(request)
 var frame envelope
 if err:=json.Unmarshal(resp,&frame);err!=nil {t.Fatal(err)}
 if !frame.OK{t.Fatal("auth parse failed")}
 var decoded map[string]any
 if err:=json.Unmarshal(frame.Result,&decoded);err!=nil{t.Fatal(err)}
 if decoded["Handled"]!=true{t.Fatal("own auth marker not recognized")}
 auth:=decoded["Auth"].(map[string]any)
 if auth["Provider"]!=pluginName||auth["ID"]!=pluginName{t.Fatal("wrong host provider identity")}
 if strings.Contains(string(resp),"secret-credential"){t.Fatal("unexpected leaked key")}
 other,_:=json.Marshal(authFileRequest{
  RawJSON: []byte(`{"type":"codex","token":"secret-credential"}`),
 })
 foreign:=parseHostAuth(other)
 if err:=json.Unmarshal(foreign,&frame);err!=nil{t.Fatal(err)}
 if err:=json.Unmarshal(frame.Result,&decoded);err!=nil{t.Fatal(err)}
 if decoded["Handled"]!=false{t.Fatal("should not intercept foreign auth")}
}
