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
 if json.Unmarshal(raw, &marker) != nil { return nil, false }
 if marker["type"] != pluginName { return nil, false }
 label, _ := marker["label"].(string)
 if strings.TrimSpace(label) == "" { label = "OFM Provider" }
 if filename == "" { filename = "ofm-provider.json" }
 record := map[string]any{
  "Provider": pluginName,
  "ID": pluginName,
  "FileName": filename,
  "Label": label,
  "StorageJSON": raw,
  "Metadata": map[string]any{"type": pluginName},
 }
 output, _ := json.Marshal(record)
 return output, true
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
