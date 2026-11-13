package main

// Account Gateway Bridge is a CLIENT of the original Our Free Model standalone
// runtime. That runtime—not this plugin—owns OAuth, account refresh, quotas,
// provider wire protocols, and account pool selection. No upstream passwords,
// browser sessions, or device identity logic are copied into CLIProxyAPI.

import (
    "encoding/json"
    "errors"
    "fmt"
    "net"
    "net/url"
    "os"
    "path/filepath"
    "sort"
    "strings"
    "sync"
)

const accountModelPrefix = "ofm-account/"
const accountSecretID = "__ofm_standalone_gateway__"

var accountChannels = map[string]string{
    "lobsterai": "LobsterAI",
    "qoder": "Qoder",
    "qoder-cn": "Qoder 中国版",
    "qoder_cn": "Qoder 中国版",
    "trae": "TRAE",
    "cline": "Cline",
    "loomy": "Loomy",
    "raccoon": "Raccoon",
    "minimax": "MiniMax Code",
    "minimax-code": "MiniMax Code",
    "zcode": "ZCode",
    "gemini": "Gemini Code Assist",
}

type accountModel struct {
    ID string `json:"id"`
    Name string `json:"name,omitempty"`
    OwnedBy string `json:"owned_by,omitempty"`
    ContextWindow int64 `json:"context_window,omitempty"`
    Vision bool `json:"vision,omitempty"`
}
type accountGatewayConfig struct {
    Enabled bool `json:"enabled"`
    BaseURL string `json:"base_url"`
    Models []accountModel `json:"models"`
}
type gatewayModelCatalog struct {
    Data []accountModel `json:"data"`
}
type accountGatewayRoute struct {
    UpstreamModel string
    DisplayName string
    ContextWindow int64
    Vision bool
}
var accounts = struct {
    sync.RWMutex
    Config accountGatewayConfig
    Routes map[string]accountGatewayRoute
}{Routes: map[string]accountGatewayRoute{}}

func defaultAccountConfig() accountGatewayConfig {
    return accountGatewayConfig{BaseURL: "http://127.0.0.1:18900", Models: []accountModel{}}
}
func safeGatewayURL(raw string) (string, error) {
    raw = strings.TrimSpace(raw)
    parsed, err := url.Parse(raw)
    if err != nil || parsed == nil || parsed.Scheme != "http" ||
        parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" ||
        parsed.Fragment != "" || parsed.Opaque != "" ||
        (parsed.Path != "" && parsed.Path != "/") {
        return "", errors.New("Account Gateway must be a local HTTP loopback origin")
    }
    host := parsed.Hostname()
    if host != "localhost" {
        ip := net.ParseIP(host)
        if ip == nil || !ip.IsLoopback() { return "", errors.New("account gateway must use localhost / 127.0.0.1 / ::1") }
    }
    if parsed.Port() == "" { return "", errors.New("specify the standalone runtime's real listening port") }
    return strings.TrimRight(parsed.String(), "/"), nil
}
func gatewayConfigPath() (string, error) {
    base, err := configPath()
    if err != nil { return "", err }
    return filepath.Join(filepath.Dir(base), "ofm-account-gateway.json"), nil
}
func readGatewayConfigUnlocked() (accountGatewayConfig, error) {
    path, err := gatewayConfigPath()
    if err != nil { return accountGatewayConfig{}, err }
    raw, err := os.ReadFile(path)
    if errors.Is(err, os.ErrNotExist) { return defaultAccountConfig(), nil }
    if err != nil { return accountGatewayConfig{}, err }
    var cfg accountGatewayConfig
    if err := json.Unmarshal(raw, &cfg); err != nil { return accountGatewayConfig{}, err }
    if _, err := safeGatewayURL(cfg.BaseURL); err != nil { return accountGatewayConfig{}, err }
    if cfg.Models == nil { cfg.Models = []accountModel{} }
    return cfg, nil
}
func writeGatewayConfigUnlocked(cfg accountGatewayConfig) error {
    if _, err := safeGatewayURL(cfg.BaseURL); err != nil { return err }
    if len(cfg.Models) > 1024 { return errors.New("too many models") }
    raw, err := json.MarshalIndent(cfg, "", "  ")
    if err != nil { return err }
    path, err := gatewayConfigPath()
    if err != nil { return err }
    return writePrivate(path, append(raw, '\n'))
}
func accountKeyUnlocked(secrets map[string]string) string {
    return strings.TrimSpace(secrets[accountSecretID])
}
func gatewayKey() (string, error) {
    configMu.Lock()
    defer configMu.Unlock()
    secrets, err := readSecretsUnlocked()
    if err != nil { return "", err }
    if key := accountKeyUnlocked(secrets); key != "" { return key, nil }
    return "", errors.New("standalone runtime's forwardKey has not been configured")
}
func accountChannelID(modelID string) (string, bool) {
    first, _, hasSlash := strings.Cut(modelID, "/")
    if !hasSlash { return "", false }
    _, ok := accountChannels[strings.ToLower(first)]
    return strings.ToLower(first), ok
}
func filterChannelModels(catalog gatewayModelCatalog) ([]accountModel, error) {
    if len(catalog.Data) > 5000 { return nil, errors.New("too many gateway model entries") }
    seen := map[string]bool{}
    selected := make([]accountModel, 0, len(catalog.Data))
    for _, m := range catalog.Data {
        if m.ID == "" || len(m.ID) > 300 || strings.ContainsAny(m.ID, "\r\n\t") { continue }
        if _, ok := accountChannelID(m.ID); !ok { continue }
        if seen[m.ID] { continue }
        seen[m.ID] = true
        if m.Name == "" { m.Name = m.ID }
        selected = append(selected, m)
    }
    sort.Slice(selected,func(i,j int)bool{return selected[i].ID < selected[j].ID})
    if len(selected)>1024 { return nil, errors.New("too many account channel models") }
    return selected,nil
}
func modelRoutes(cfg accountGatewayConfig) map[string]accountGatewayRoute {
    result := map[string]accountGatewayRoute{}
    if !cfg.Enabled { return result }
    for _, m := range cfg.Models {
        if _, ok := accountChannelID(m.ID); !ok { continue }
        result[accountModelPrefix+m.ID] = accountGatewayRoute{
            UpstreamModel: m.ID, DisplayName: m.Name,
            ContextWindow: m.ContextWindow, Vision: m.Vision,
        }
    }
    return result
}
func replaceAccounts(cfg accountGatewayConfig) {
    accounts.Lock()
    accounts.Config = cfg
    accounts.Routes = modelRoutes(cfg)
    accounts.Unlock()
}
func loadAccounts() error {
    configMu.Lock()
    cfg, err := readGatewayConfigUnlocked()
    configMu.Unlock()
    if err != nil { return fmt.Errorf("account gateway config: %w",err) }
    replaceAccounts(cfg)
    return nil
}
func accountRoutesSnapshot() (accountGatewayConfig,map[string]accountGatewayRoute) {
    accounts.RLock()
    defer accounts.RUnlock()
    routes:=make(map[string]accountGatewayRoute,len(accounts.Routes))
    for k,v:=range accounts.Routes { routes[k]=v }
    return accounts.Config,routes
}
func accountResolve(model string) (accountGatewayConfig, accountGatewayRoute,bool) {
    accounts.RLock()
    defer accounts.RUnlock()
    route,ok:=accounts.Routes[model]
    return accounts.Config,route,ok
}
