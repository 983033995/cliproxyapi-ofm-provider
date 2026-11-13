package main

import (
 "encoding/json"
 "strings"
)

// This marker establishes a host-side credential SELECTOR, not vendor OAuth.
// Vendor API Keys remain in the separate local file or environment and are
// never copied to the host's auth files.
type authFileRequest struct {
 Provider string
 FileName string
 RawJSON []byte
}
type authRefreshRequest struct {
 AuthID string
 StorageJSON []byte
}
func authPayload(raw []byte, filename string) ([]byte, bool) {
 var marker map[string]any
 if json.Unmarshal(raw, &marker) != nil || marker["type"] != pluginName {
  return nil, false
 }
 id := pluginName
 label,_:=marker["label"].(string)
 if strings.TrimSpace(label)=="" {label="OFM Provider"}
 metadata:=map[string]any{"type":pluginName}
 attributes:=map[string]string{}
 if accountID,ok:=marker["account_id"].(string);ok && accountID!="" {
  if !validAccountID(accountID) {return nil,false}
  portal,_:=marker["oauth_provider"].(string)
  if _,known:=nativeChannel(portal);!known {return nil,false}
  id=pluginName+"-"+accountID
  if filename==""||filename=="ofm-provider.json" {filename=id+".json"}
  metadata["oauth_provider"]=portal
  attributes["oauth_provider"]=portal
 }
 if filename=="" {filename="ofm-provider.json"}
 record := map[string]any{
  "Provider":pluginName,
  "ID":id,
  "FileName":filename,
  "Label":label,
  "StorageJSON":raw,
  "Metadata":metadata,
 }
 if len(attributes)>0 { record["Attributes"]=attributes }
 output,_:=json.Marshal(record)
 return output,true
}
func parseHostAuth(raw []byte) []byte {
 var req authFileRequest
 if err := json.Unmarshal(raw, &req); err != nil { return fail("invalid_request", "invalid host auth parse request", 400) }
 record, handled := authPayload(req.RawJSON, req.FileName)
 if !handled { return ok(map[string]any{"Handled": false}) }
 return ok(map[string]any{"Handled": true, "Auth": json.RawMessage(record)})
}
func refreshHostAuth(raw []byte) []byte {
 var req authRefreshRequest
 if json.Unmarshal(raw, &req) != nil { return fail("invalid_request", "invalid auth refresh request", 400) }
 record, handled := authPayload(req.StorageJSON, "ofm-provider.json")
 if !handled { return fail("invalid_request", "invalid host auth marker", 400) }
 return ok(map[string]any{"Auth": json.RawMessage(record)})
}
