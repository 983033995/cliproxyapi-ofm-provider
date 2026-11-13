package provider

import (
  "encoding/json"
  "errors"
  "fmt"
  "net"
  "net/url"
  "os"
  "sort"
  "strings"
)

const Chat = "openai-chat"

type Model struct {
  ID string `json:"id"`
  Name string `json:"name,omitempty"`
  ContextWindow int64 `json:"context_window,omitempty"`
  Vision bool `json:"vision,omitempty"`
}
type Entry struct {
  ID string `json:"id"`
  Name string `json:"name"`
  Enabled bool `json:"enabled"`
  BaseURL string `json:"base_url"`
  Format string `json:"format"`
  AuthKind string `json:"auth_kind"`
  KeyEnv string `json:"key_env,omitempty"`
  Headers map[string]string `json:"headers,omitempty"`
  Models []Model `json:"models"`
}
type Config struct { Providers []Entry `json:"providers"` }
type Route struct { Provider Entry; Model Model }
type Registry struct { Routes map[string]Route; Entries []Entry }

func SafeURL(raw string, local bool) error {
  u, err := url.Parse(strings.TrimSpace(raw))
  if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
    return errors.New("base_url must be an absolute URL without userinfo/query/fragment")
  }
  if strings.HasSuffix(u.Path, "/chat/completions") || strings.HasSuffix(u.Path, "/responses") || strings.HasSuffix(u.Path, "/messages") {
    return errors.New("base_url must not include a completion endpoint")
  }
  if u.Scheme == "https" { return nil }
  if u.Scheme != "http" || !local { return errors.New("remote upstreams require HTTPS") }
  if u.Hostname() == "localhost" { return nil }
  if ip := net.ParseIP(u.Hostname()); ip != nil && ip.IsLoopback() { return nil }
  return errors.New("keyless HTTP services must be local loopback")
}

func LoadFile(path string) (Registry, error) {
  if strings.TrimSpace(path) == "" { return Registry{Routes: map[string]Route{}}, nil }
  raw, err := os.ReadFile(path)
  if err != nil { return Registry{}, err }
  var cfg Config
  if err := json.Unmarshal(raw, &cfg); err != nil { return Registry{}, fmt.Errorf("invalid provider config: %w", err) }
  return Validate(cfg)
}

func Validate(cfg Config) (Registry, error) {
  result := Registry{Routes: make(map[string]Route)}
  seen := map[string]bool{}
  for _, p := range cfg.Providers {
    if p.ID == "" || strings.ContainsAny(p.ID, "/ ?&#") || seen[p.ID] {
      return Registry{}, fmt.Errorf("invalid or duplicate provider id %q", p.ID)
    }
    seen[p.ID] = true
    if p.Format != Chat {
      if p.Enabled { return Registry{}, fmt.Errorf("provider %s: unsupported wire %q", p.ID, p.Format) }
      continue
    }
    if p.AuthKind != "apiKey" && p.AuthKind != "none" {
      if p.Enabled { return Registry{}, fmt.Errorf("provider %s: unsupported auth %q", p.ID, p.AuthKind) }
      continue
    }
    if !p.Enabled { continue }
    if err := SafeURL(p.BaseURL, p.AuthKind == "none"); err != nil {
      return Registry{}, fmt.Errorf("provider %s: %w", p.ID, err)
    }
    if p.AuthKind == "apiKey" && strings.TrimSpace(p.KeyEnv) == "" {
      return Registry{}, fmt.Errorf("provider %s: key_env is required", p.ID)
    }
    for k, v := range p.Headers {
      lower := strings.ToLower(strings.TrimSpace(k))
      if lower == "authorization" || lower == "x-api-key" || lower == "host" ||
        lower == "content-length" || lower == "content-type" || strings.ContainsRune(k+v, rune(13)) || strings.ContainsRune(k+v, rune(10)) ||
        strings.Contains(k, ":") {
        return Registry{}, fmt.Errorf("provider %s: forbidden header %q", p.ID, k)
      }
    }
    if len(p.Models) == 0 { return Registry{}, fmt.Errorf("provider %s: no models", p.ID) }
    for _, m := range p.Models {
      if m.ID == "" || strings.TrimSpace(m.ID) != m.ID {
        return Registry{}, fmt.Errorf("provider %s: invalid model id", p.ID)
      }
      exposed := "ofm/" + p.ID + "/" + m.ID
      if _, ok := result.Routes[exposed]; ok { return Registry{}, fmt.Errorf("duplicate model route %q", exposed) }
      result.Routes[exposed] = Route{Provider: p, Model: m}
    }
    result.Entries = append(result.Entries, p)
  }
  sort.Slice(result.Entries, func(i, j int) bool { return result.Entries[i].ID < result.Entries[j].ID })
  return result, nil
}

func (r Registry) ModelIDs() []string {
  ids := make([]string, 0, len(r.Routes))
  for id := range r.Routes { ids = append(ids, id) }
  sort.Strings(ids)
  return ids
}
func (r Registry) Resolve(id string) (Route, bool) { v, ok := r.Routes[id]; return v, ok }
func Endpoint(p Entry) string { return strings.TrimRight(p.BaseURL, "/") + "/chat/completions" }

func Key(p Entry) (string, error) {
  if p.AuthKind == "none" { return "", nil }
  key := os.Getenv(p.KeyEnv)
  if strings.TrimSpace(key) == "" { return "", errors.New("credential unavailable") }
  return key, nil
}
func Payload(source []byte, nativeModel string, stream bool) ([]byte, error) {
  var input map[string]json.RawMessage
  if err := json.Unmarshal(source, &input); err != nil || input == nil { return nil, errors.New("expected JSON object") }
  model, _ := json.Marshal(nativeModel)
  streaming, _ := json.Marshal(stream)
  input["model"] = model
  input["stream"] = streaming
  return json.Marshal(input)
}
