package provider

import (
  "encoding/json"
  "testing"
)

func sample() Config {
  return Config{Providers: []Entry{{
    ID: "acme", Name: "Acme", Enabled: true, BaseURL: "https://example.org/v1",
    Format: Chat, AuthKind: "apiKey", KeyEnv: "ACME_KEY",
    Models: []Model{{ID: "org/deep/model", Name: "Nested"}},
  }}}
}
func TestCompositeModelIDsPreserveSlashes(t *testing.T) {
  r, err := Validate(sample())
  if err != nil { t.Fatal(err) }
  v, ok := r.Resolve("ofm/acme/org/deep/model")
  if !ok || v.Model.ID != "org/deep/model" { t.Fatalf("nested model lost: %#v", v) }
}
func TestRejectUnsupportedWire(t *testing.T) {
  c := sample()
  c.Providers[0].Format = "anthropic-messages"
  if _, err := Validate(c); err == nil { t.Fatal("should not silently convert Anthropic wire") }
  c.Providers[0].Enabled = false
  if _, err := Validate(c); err != nil { t.Fatal(err) }
}
func TestRejectInsecureOrCredentialLeakingConfiguration(t *testing.T) {
  c := sample()
  c.Providers[0].BaseURL = "http://remote.example/v1"
  if _, err := Validate(c); err == nil { t.Fatal("unencrypted remote allowed") }
  c = sample()
  c.Providers[0].Headers = map[string]string{"Authorization": "secret"}
  if _, err := Validate(c); err == nil { t.Fatal("Authorization header override allowed") }
  c = sample()
  c.Providers[0].BaseURL = "https://user:pass@example.org/v1"
  if _, err := Validate(c); err == nil { t.Fatal("embedded credentials allowed") }
}
func TestPayloadRewritesNativeModelWithoutDroppingTools(t *testing.T) {
  input := []byte(`{"model":"ofm/acme/org/model","messages":[{"role":"user","content":"hi"}],"tools":[{"type":"function","function":{"name":"search","parameters":{}}}]}`)
  result, err := Payload(input, "org/model", true)
  if err != nil { t.Fatal(err) }
  var body map[string]any
  if err := json.Unmarshal(result, &body); err != nil { t.Fatal(err) }
  if body["model"] != "org/model" || body["stream"] != true || body["tools"] == nil {
    t.Fatalf("unexpected payload: %s", result)
  }
}
func TestLocalhostHTTPOnlyForNoKeyService(t *testing.T) {
  c := sample()
  c.Providers[0].AuthKind = "none"
  c.Providers[0].KeyEnv = ""
  c.Providers[0].BaseURL = "http://127.0.0.1:11434/v1"
  if _, err := Validate(c); err != nil { t.Fatal(err) }
  c.Providers[0].BaseURL = "http://192.168.1.2:11434/v1"
  if _, err := Validate(c); err == nil { t.Fatal("unsafe plaintext LAN provider allowed") }
}
func TestDisabledProviderNotRegistered(t *testing.T) {
  c := sample()
  c.Providers[0].Enabled = false
  r, err := Validate(c)
  if err != nil { t.Fatal(err) }
  if len(r.ModelIDs()) != 0 { t.Fatal("disabled model was registered") }
}
