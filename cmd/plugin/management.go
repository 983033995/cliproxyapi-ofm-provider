package main

import (
    _ "embed"
    "encoding/json"
    "errors"
    "fmt"
    "net/http"
    "net/url"
    "strings"

    "github.com/983033995/cliproxyapi-ofm-provider/internal/provider"
)

//go:embed web/catalog.json
var catalogJSON []byte
//go:embed web/index.html
var dashboardHTML []byte
//go:embed web/app.css
var dashboardCSS []byte
//go:embed web/app.js
var dashboardJS []byte

type managementRequest struct {
    Method string
    Path string
    Body []byte
    Headers http.Header
    Query url.Values
    HostCallbackID string `json:"host_callback_id,omitempty"`
}

type providerMutation struct {
    ID string `json:"id"`
    Enabled *bool `json:"enabled,omitempty"`
    BaseURL string `json:"base_url"`
    KeyEnv string `json:"key_env"`
    APIKey string `json:"api_key,omitempty"`
    Models []provider.Model `json:"models,omitempty"`
    ConfirmURLChange bool `json:"confirm_url_change,omitempty"`
}

type providerTest struct {
    ID string `json:"id"`
    BaseURL string `json:"base_url"`
    APIKey string `json:"api_key"`
    HostCallbackID string
}

func registerManagement() []byte {
    routes := []map[string]any{}
    for _, spec := range []struct{ method, path string }{
        {"GET", "/ofm-provider/providers"},
        {"POST", "/ofm-provider/providers/save"},
        {"POST", "/ofm-provider/providers/test"},
        {"POST", "/ofm-provider/providers/disconnect"},
    } {
        routes = append(routes, map[string]any{"Method": spec.method, "Path": spec.path})
    }
    resources := []map[string]any{
        {"Path": "/dashboard", "Menu": "第三方模型接入", "Description": "第三方模型供应商账号与 API Key 管理"},
        {"Path": "/app.css"},
        {"Path": "/app.js"},
    }
    return ok(map[string]any{"routes": routes, "resources": resources})
}

func managementResponse(status int, contentType string, body []byte) []byte {
    return ok(map[string]any{
        "StatusCode": status,
        "Headers": http.Header{
            "Content-Type": {contentType},
            "Cache-Control": {"no-store"},
            "X-Content-Type-Options": {"nosniff"},
            "Referrer-Policy": {"no-referrer"},
            "X-Frame-Options": {"SAMEORIGIN"},
            "Content-Security-Policy": {"default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self' data:; base-uri 'none'; form-action 'none'"},
        },
        "Body": body,
    })
}
func managementJSON(status int, value any) []byte {
    body, _ := json.Marshal(value)
    return managementResponse(status, "application/json; charset=utf-8", body)
}
func managementError(status int, code, detail string) []byte {
    return managementJSON(status, map[string]any{"ok": false, "code": code, "message": detail})
}

func handleManagement(raw []byte) []byte {
    var req managementRequest
    if err := json.Unmarshal(raw, &req); err != nil { return managementError(400, "bad_request", "管理请求格式错误") }
    switch req.Method + " " + req.Path {
    // Resource routes are UNAUTHENTICATED in the host. Expose only static files,
    // never configuration, keys, account status, or upstream callbacks here.
    case "GET /v0/resource/plugins/ofm-provider/dashboard":
        return managementResponse(200, "text/html; charset=utf-8", dashboardHTML)
    case "GET /v0/resource/plugins/ofm-provider/app.css":
        return managementResponse(200, "text/css; charset=utf-8", dashboardCSS)
    case "GET /v0/resource/plugins/ofm-provider/app.js":
        return managementResponse(200, "text/javascript; charset=utf-8", dashboardJS)
    case "GET /v0/management/ofm-provider/providers":
        return listProviderResponse()
    case "POST /v0/management/ofm-provider/providers/save":
        return saveProviderResponse(req.Body)
    case "POST /v0/management/ofm-provider/providers/disconnect":
        return disconnectProviderResponse(req.Body)
    case "POST /v0/management/ofm-provider/providers/test":
        return testProviderResponse(req.Body, req.HostCallbackID)
    default:
        return managementError(404, "not_found", "未知管理接口")
    }
}

func providerState(p provider.Entry, secrets map[string]string) map[string]any {
    _, from := credentialUnlocked(p, secrets)
    supported := p.Format == provider.Chat && (p.AuthKind == "apiKey" || p.AuthKind == "none")
    return map[string]any{
        "id": p.ID, "name": p.Name, "enabled": p.Enabled,
        "base_url": p.BaseURL, "format": p.Format,
        "auth_kind": p.AuthKind, "key_env": p.KeyEnv,
        "models": p.Models,
        "supported": supported,
        "connected": (from != "" && p.Enabled),
        "credential_present": from != "",
        "credential_source": from,
    }
}

func listProviderResponse() []byte {
    configMu.Lock()
    defer configMu.Unlock()
    cfg, err := readConfigUnlocked()
    if err != nil { return managementError(500, "config_error", "无法读取供应商配置") }
    secrets, err := readSecretsUnlocked()
    if err != nil { return managementError(500, "secrets_error", err.Error()) }
    out := make([]map[string]any, 0, len(cfg.Providers))
    for _, p := range cfg.Providers { out = append(out, providerState(p, secrets)) }
    return managementJSON(200, map[string]any{"ok": true, "providers": out,
        "note": "已保存配置；新增/更改模型目录可能需要重载 CLIProxyAPI 宿主才会生效"})
}

func updateProvider(cfg *provider.Config, id string) (*provider.Entry, error) {
    for i := range cfg.Providers {
        if cfg.Providers[i].ID == id { return &cfg.Providers[i], nil }
    }
    return nil, errors.New("找不到供应商；请使用内置目录中的供应商")
}

func saveProviderResponse(body []byte) []byte {
    if len(body) > 64 << 10 { return managementError(413, "too_large", "请求内容过大") }
    var req providerMutation
    if err := json.Unmarshal(body, &req); err != nil { return managementError(400, "bad_request", "配置 JSON 无效") }
    if len(req.APIKey) > 8192 || strings.ContainsAny(req.APIKey, "\r\n") {
        return managementError(400, "invalid_key", "API Key 格式无效")
    }
    configMu.Lock()
    defer configMu.Unlock()
    cfg, err := readConfigUnlocked()
    if err != nil { return managementError(500, "config_error", "读取配置失败") }
    item, err := updateProvider(&cfg, req.ID)
    if err != nil { return managementError(404, "unknown_provider", err.Error()) }
    oldURL := item.BaseURL
    if req.BaseURL != "" { item.BaseURL = strings.TrimRight(strings.TrimSpace(req.BaseURL), "/") }
    if req.KeyEnv != "" { item.KeyEnv = strings.TrimSpace(req.KeyEnv) }
    if req.Enabled != nil { item.Enabled = *req.Enabled }
    if req.Models != nil {
        if len(req.Models) > 500 { return managementError(400, "too_many_models", "模型数量不能超过 500") }
        item.Models = req.Models
    }
    if item.BaseURL != oldURL && !req.ConfirmURLChange {
        return managementError(409, "confirm_endpoint_change", "更改 API 地址前需确认 Key 可能发送至新地址")
    }
    if item.Enabled {
        if item.Format != provider.Chat || (item.AuthKind != "apiKey" && item.AuthKind != "none") {
            return managementError(400, "unsupported", "该供应商需要额外的协议或授权适配器，目前无法启用")
        }
    }
    secrets, err := readSecretsUnlocked()
    if err != nil { return managementError(500, "secrets_error", "凭据文件读取失败") }
    if item.BaseURL != oldURL { delete(secrets, item.ID) }
    if req.APIKey != "" {
        if item.AuthKind != "apiKey" { return managementError(400, "unsupported", "本地免密供应商不需要 API Key") }
        secrets[item.ID] = req.APIKey
    }
    if item.Enabled && item.AuthKind == "apiKey" {
        key, _ := credentialUnlocked(*item, secrets)
        if key == "" { return managementError(400, "missing_credential", "先提供 API Key 或配置有效的环境变量") }
    }
    if _, err := provider.Validate(cfg); err != nil {
        return managementError(400, "invalid_provider", err.Error())
    }
    if err := writeSecretsUnlocked(secrets); err != nil {
        return managementError(500, "save_error", "无法安全保存凭据（需要 0600 权限）")
    }
    if err := writeConfigUnlocked(cfg); err != nil { return managementError(500, "save_error", "无法保存供应商配置") }
    registered, err := provider.Validate(cfg)
    if err == nil {
        runtime.Lock()
        runtime.registry = registered
        runtime.Unlock()
    }
    return managementJSON(200, map[string]any{"ok": true,
        "message": "配置已保存；如果模型目录未更新，请重载 CLIProxyAPI 插件或重启宿主",
        "provider": providerState(*item, secrets), "reload_required": true})
}

func disconnectProviderResponse(body []byte) []byte {
    var req struct { ID string `json:"id"` }
    if err := json.Unmarshal(body, &req); err != nil { return managementError(400, "bad_request", "格式错误") }
    configMu.Lock()
    defer configMu.Unlock()
    cfg, err := readConfigUnlocked()
    if err != nil { return managementError(500, "config_error", "读取配置失败") }
    item, err := updateProvider(&cfg, req.ID)
    if err != nil { return managementError(404, "unknown_provider", err.Error()) }
    item.Enabled = false
    secrets, err := readSecretsUnlocked()
    if err != nil { return managementError(500, "secrets_error", "读取凭据失败") }
    delete(secrets, req.ID)
    if err := writeSecretsUnlocked(secrets); err != nil { return managementError(500, "save_error", "移除凭据失败") }
    if err := writeConfigUnlocked(cfg); err != nil { return managementError(500, "save_error", "保存配置失败") }
    registered, _ := provider.Validate(cfg)
    runtime.Lock()
    runtime.registry = registered
    runtime.Unlock()
    return managementJSON(200, map[string]any{"ok": true,
        "message": "已禁用供应商并清除插件本地保存的 Key；如使用环境变量，还需在宿主环境中删除该变量"})
}

func testProviderResponse(body []byte, callbackID string) []byte {
    var req providerTest
    if len(body) > 64 << 10 { return managementError(413, "too_large", "请求过大") }
    if err := json.Unmarshal(body, &req); err != nil { return managementError(400, "bad_request", "格式错误") }
    if len(req.APIKey) > 8192 || strings.ContainsAny(req.APIKey, "\r\n") {
        return managementError(400, "invalid_key", "API Key 格式无效")
    }
    configMu.Lock()
    cfg, err := readConfigUnlocked()
    if err != nil { configMu.Unlock(); return managementError(500, "config_error", "读取配置失败") }
    item, err := updateProvider(&cfg, req.ID)
    if err != nil { configMu.Unlock(); return managementError(404, "unknown_provider", err.Error()) }
    p := *item
    if req.BaseURL != "" { p.BaseURL = strings.TrimRight(strings.TrimSpace(req.BaseURL), "/") }
    if p.Format != provider.Chat || (p.AuthKind != "apiKey" && p.AuthKind != "none") {
        configMu.Unlock()
        return managementError(400, "unsupported", "该账号/协议渠道暂未实现连接测试")
    }
    if err := provider.SafeURL(p.BaseURL, p.AuthKind == "none"); err != nil {
        configMu.Unlock()
        return managementError(400, "invalid_url", err.Error())
    }
    secrets, err := readSecretsUnlocked()
    if err != nil { configMu.Unlock(); return managementError(500, "secrets_error", "读取凭据失败") }
    key, _ := credentialUnlocked(p, secrets)
    if p.BaseURL != item.BaseURL { key = "" }
    if req.APIKey != "" { key = req.APIKey }
    configMu.Unlock()
    if p.AuthKind == "apiKey" && key == "" {
        return managementError(400, "missing_credential", "请输入 API Key 再测试")
    }
    headers := http.Header{"Accept": {"application/json"}}
    if key != "" { headers.Set("Authorization", "Bearer " + key) }
    var upstream httpResp
    err = hostCall("host.http.do", map[string]any{
        "host_callback_id": callbackID, "method": "GET",
        "url": p.BaseURL + "/models", "headers": headers,
    }, &upstream)
    if err != nil { return managementError(502, "transport_error", "连接失败，请检查网络与 API 地址") }
    if upstream.StatusCode < 200 || upstream.StatusCode >= 300 {
        // Never echo the raw response: some upstream providers reflect credentials.
        return managementError(400, "upstream_error", fmt.Sprintf("供应商返回 HTTP %d，检查 Key、额度及模型权限", upstream.StatusCode))
    }
    var listing struct { Data []json.RawMessage `json:"data"` }
    if len(upstream.Body) > 0 { _ = json.Unmarshal(upstream.Body, &listing) }
    return managementJSON(200, map[string]any{
        "ok": true, "message": "模型目录 API 访问成功（并不代表对话生成已通过）",
        "listed_models": len(listing.Data)})
}
