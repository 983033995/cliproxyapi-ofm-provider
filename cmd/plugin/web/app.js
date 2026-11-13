"use strict";

(function () {
  const $ = (id) => document.getElementById(id);
  const path = "/v0/management/ofm-provider/providers";
  const model = {key: "", providers: [], filter: "all", query: "", tab: "providers", selected: null, gateway: null};
  const secure = location.protocol === "https:" || ["127.0.0.1", "localhost", "::1", "[::1]"].includes(location.hostname);
  const accountChannels = [
    ["LobsterAI", "有道", "需要账号授权及积分接口"],
    ["Qoder", "阿里系", "需要账号授权及积分接口"],
    ["Qoder 中国版", "阿里系", "需要账号授权及积分接口"],
    ["TRAE", "字节", "需要独立客户端账号适配"],
    ["Cline", "Cline", "需要独立账号授权适配"],
    ["Loomy", "讯飞", "需要独立账号授权适配"],
    ["Raccoon", "商汤", "需要独立账号授权适配"],
    ["MiniMax Code", "MiniMax", "需要官方授权与额度接口"],
    ["ZCode", "智谱", "需要独立账号授权适配"],
    ["Gemini Code Assist", "Google", "需要独立 OAuth 授权适配"]
  ];

  const clear = (node) => { while (node.firstChild) node.removeChild(node.firstChild); };
  const el = (tag, cls, text) => {
    const node = document.createElement(tag);
    if (cls) node.className = cls;
    if (text !== undefined) node.textContent = text;
    return node;
  };
  const label = (kind) => ({
    apiKey: "API Key", none: "本地免密", oauth: "OAuth 授权", adc: "云端身份 ADC"
  }[kind] || kind || "未知认证");
  function apiUrl(action) { return action ? path + "/" + action : path; }
  function message(id, content) { $(id).textContent = content || ""; }
  function showLogin(error) {
    model.key = "";
    $("management-key").value = "";
    message("credentials-error", error || "");
    if (!$("credentials").open) $("credentials").showModal();
  }
  async function request(action, options) {
    if (!secure) throw Error("远程管理需要 HTTPS。请在本机回环地址或 HTTPS 下打开此页面。");
    const headers = {"X-Management-Key": model.key, "Accept": "application/json"};
    let method = "GET", body;
    if (options) {
      method = "POST";
      body = JSON.stringify(options);
      headers["Content-Type"] = "application/json";
    }
    const response = await fetch(apiUrl(action), {
      method, headers, body, credentials: "same-origin",
      cache: "no-store", redirect: "error"
    });
    if (response.status === 401 || response.status === 403) {
      showLogin("管理密钥无效，或当前地址不允许访问管理 API。");
      throw Error("管理 API 未授权");
    }
    let data;
    try { data = await response.json(); } catch (_) { throw Error("管理接口未返回有效 JSON。请确认 CLIProxyAPI 支持当前插件。"); }
    if (!response.ok || data.ok !== true) throw Error(data.message || "请求失败 (HTTP " + response.status + ")");
    return data;
  }
  async function refresh() {
    const data = await request("");
    model.providers = Array.isArray(data.providers) ? data.providers : [];
    $("total").textContent = model.providers.length;
    $("active").textContent = model.providers.filter((p) => p.enabled && p.connected).length;
    $("available").textContent = model.providers.filter((p) => p.supported).length;
    $("tab-count").textContent = String(model.providers.length);
    renderCards();
    try { await refreshGateway(); } catch (err) { message("gateway-error", err.message); }
  }
  function badges(p) {
    if (!p.supported) return ["待适配", "tag unsupported"];
    if (p.enabled && p.connected) return ["已接入", "tag live"];
    if (p.enabled) return ["待验证", "tag"];
    return ["待接入", "tag"];
  }
  function card(p, planned) {
    const card = el("article", "provider-card");
    if (p.enabled && p.connected) card.classList.add("is-live");
    if ((!p.supported && !planned) || (planned && !(p.models || []).length)) card.classList.add("is-unsupported");

    const top = el("div", "card-top");
    const ident = el("div", "card-identity");
    const tone = Array.from(p.id).reduce((n, c) => n + c.charCodeAt(0), 0) % 4;
    const avatar = el("div", "avatar", (p.name || p.id).slice(0, 1));
    avatar.dataset.tone = String(tone);
    const txt = el("div");
    txt.style.minWidth = "0";
    txt.append(el("h3", "", p.name || p.id), el("div", "subheading", planned ? p.owner : p.id));
    ident.append(avatar, txt);
    const status = badges(p);
    if (planned) { status[0] = (p.models || []).length ? "已同步模型" : "待登录/同步"; status[1] = (p.models || []).length ? "tag live" : "tag"; }
    const tag = el("span", status[1], status[0]);
    top.append(ident, tag);

    const desc = el("p", "card-description", planned ?
      p.description : label(p.auth_kind) + " · " + (p.format || "未知协议") + " · " + (p.base_url || "未设置接入地址"));
    const metrics = el("div", "card-metrics");
    const modelCount = el("span", "", "模型 ");
    modelCount.append(el("b", "", String((p.models || []).length)));
    metrics.append(modelCount);
    if (!planned) {
      const key = el("span", "", "凭据 ");
      key.append(el("b", p.credential_present ? "ok" : "", p.auth_kind === "none" ? "不需要" :
        (p.credential_present ? "已配置" : "未配置")));
      metrics.append(key);
    }
    const actions = el("div", "card-actions");
    const button = el("button", p.supported && !planned ? "accent" : "", planned ? "打开账号授权" : p.supported ?
      (p.enabled ? "管理接入" : "接入设置") : "待适配");
    button.type = "button";
    if (planned) button.addEventListener("click", openAccountConsole);
    else if (p.supported) button.addEventListener("click", () => openEditor(p.id));
    else {
      button.disabled = true;
      button.title = "需要独立的协议或账号授权适配器，暂不能使用";
    }
    const tip = el("small", "", planned ? "账号与额度由本地服务管理" :
      (p.credential_source === "local-file" ? "本地私有凭据文件" :
        (p.credential_source === "environment" ? "环境变量凭据" :
          (p.supported ? "启用后需要模型目录重载" : "需额外协议适配"))));
    actions.append(button, tip);
    card.append(top, desc, metrics, actions);
    return card;
  }

  function renderCards() {
    const box = $("providers");
    clear(box);
    const needle = model.query.trim().toLowerCase();
    const filtered = model.providers.filter((p) => {
      if (model.filter === "connected" && !(p.enabled && p.connected)) return false;
      if (model.filter === "ready" && !p.supported) return false;
      if (model.filter === "unsupported" && p.supported) return false;
      const text = (p.id + " " + p.name + " " + p.format + " " +
        (p.models || []).map((m) => m.id + " " + (m.name || "")).join(" ")).toLowerCase();
      return !needle || text.includes(needle);
    });
    filtered.forEach((p) => box.append(card(p, false)));
    $("visible-count").textContent = "显示 " + filtered.length + " / " + model.providers.length;
    $("empty").hidden = filtered.length > 0;
  }
  function renderAccounts() {
    const box = $("account-grid");
    clear(box);
    const prefixes = [
      ["lobsterai"], ["qoder"], ["qoder-cn", "qoder_cn"], ["trae"],
      ["cline"], ["loomy"], ["raccoon"], ["minimax", "minimax-code"], ["zcode"], ["gemini"]
    ];
    const synced = model.gateway?.models || [];
    accountChannels.forEach((a, index) => {
      const models = synced.filter((m) => prefixes[index].includes(String(m.id || "").split("/")[0].toLowerCase()));
      box.append(card({
        id: "account-" + index, name: a[0], owner: a[1], description: a[2],
        format: "account-channel", supported: false, enabled: models.length > 0,
        connected: models.length > 0, models
      }, true));
    });
  }

  function currentDraft() {
    const p = model.selected;
    const ids = $("models-input").value.split(/\r?\n/).map((s) => s.trim()).filter(Boolean);
    const seen = new Set();
    const models = [];
    ids.forEach((id) => {
      if (seen.has(id)) return;
      seen.add(id);
      const previous = (p.models || []).find((m) => m.id === id);
      models.push(previous || {id, name: id, context_window: 0, vision: false});
    });
    return {
      id: p.id,
      base_url: $("base-url").value.trim(),
      key_env: $("key-env").value.trim(),
      api_key: $("provider-key").value.trim(),
      models,
      confirm_url_change: false
    };
  }
  function openEditor(id) {
    const p = model.providers.find((q) => q.id === id);
    if (!p || !p.supported) return;
    model.selected = p;
    $("editor-title").textContent = p.name;
    $("editor-subtitle").textContent = label(p.auth_kind) + " · " + p.format + " · " + p.id;
    $("base-url").value = p.base_url || "";
    $("key-env").value = p.key_env || "";
    $("provider-key").value = "";
    $("key-field").hidden = p.auth_kind === "none";
    $("models-input").value = (p.models || []).map((m) => m.id).join("\n");
    $("disconnect").disabled = !p.enabled && !p.credential_present;
    message("editor-error", "");
    message("editor-success", "");
    $("editor").showModal();
  }
  function ensureEndpointConfirmation(draft) {
    if (model.selected && draft.base_url.replace(/\/+$/, "") !== (model.selected.base_url || "").replace(/\/+$/, "")) {
      if (!confirm("已修改 API 地址。原 Key 可能发送至新的服务端，旧的本地保存 Key 会清除。确定继续？")) {
        return false;
      }
      draft.confirm_url_change = true;
    }
    return true;
  }
  async function saveEditor() {
    message("editor-error", "");
    message("editor-success", "");
    const draft = currentDraft();
    draft.enabled = true;
    if (!ensureEndpointConfirmation(draft)) return;
    try {
      const data = await request("save", draft);
      $("provider-key").value = "";
      message("editor-success", data.message || "配置已保存");
      await refresh();
      model.selected = model.providers.find((p) => p.id === draft.id) || model.selected;
    } catch (err) { message("editor-error", err.message); }
  }
  async function testConnection() {
    message("editor-error", "");
    message("editor-success", "");
    const draft = currentDraft();
    if (draft.base_url !== model.selected.base_url && !draft.api_key && model.selected.auth_kind === "apiKey") {
      message("editor-error", "测试新 API 地址需要输入新 Key，原地址上的 Key 不会自动发送到新地址。");
      return;
    }
    $("test-connection").disabled = true;
    try {
      const result = await request("test", {id: draft.id, base_url: draft.base_url, api_key: draft.api_key});
      message("editor-success", (result.message || "模型目录访问成功") +
        " · 返回模型数量 " + result.listed_models);
    } catch (err) { message("editor-error", err.message); }
    finally { $("test-connection").disabled = false; }
  }
  async function disconnect() {
    const p = model.selected;
    if (!p || !confirm("确认禁用 " + p.name + " 并清除插件保存在本地的 Key 吗？")) return;
    try {
      const result = await request("disconnect", {id: p.id});
      $("provider-key").value = "";
      message("editor-success", result.message);
      await refresh();
      model.selected = model.providers.find((q) => q.id === p.id);
      $("disconnect").disabled = true;
    } catch (err) { message("editor-error", err.message); }
  }

  function checkedGatewayOrigin(raw) {
    const u = new URL(raw);
    const host = u.hostname.toLowerCase();
    if (u.protocol !== "http:" || !["127.0.0.1", "localhost", "[::1]"].includes(host) ||
        !u.port || u.username || u.password || u.search || u.hash ||
        !["", "/"].includes(u.pathname)) throw Error("仅允许同机 localhost / 127.0.0.1 的 HTTP 服务地址");
    return u.origin;
  }
  function openAccountConsole(ev) {
    try {
      const raw = $("gateway-url").value.trim() || "http://127.0.0.1:18900";
      const origin = checkedGatewayOrigin(raw);
      if (ev && ev.preventDefault) ev.preventDefault();
      window.open(origin + "/", "_blank", "noopener,noreferrer");
    } catch (err) { message("gateway-error", err.message); }
  }
  async function gatewayRequest(action, payload) {
    if (!secure) throw Error("远程账号管理需要 HTTPS");
    const headers = {"X-Management-Key": model.key, "Accept": "application/json"};
    const method = payload === undefined ? "GET" : "POST";
    if (payload !== undefined) headers["Content-Type"] = "application/json";
    const resp = await fetch("/v0/management/ofm-provider/account-gateway" + (action ? "/" + action : ""), {
      method, headers, body: payload === undefined ? undefined : JSON.stringify(payload),
      credentials: "same-origin", redirect: "error", cache: "no-store"
    });
    if (resp.status === 401 || resp.status === 403) {
      showLogin("需要重新进行 Management Key 验证");
      throw Error("管理认证失败");
    }
    let data;
    try { data = await resp.json(); } catch (err) { throw Error("账号管理接口未返回 JSON"); }
    if (!resp.ok || !data.ok) throw Error(data.message || "账号服务请求失败");
    return data;
  }
  async function refreshGateway() {
    const data = await gatewayRequest("");
    model.gateway = data;
    if (document.activeElement !== $("gateway-url")) $("gateway-url").value = data.base_url || "http://127.0.0.1:18900";
    $("gateway-status").textContent = data.enabled ? "已配置" : "未连接";
    $("gateway-status").className = data.enabled ? "tag live" : "tag";
    $("gateway-model-count").textContent = String(data.connected_models || 0);
    $("gateway-credential-status").textContent = data.credential_present ? "已配置" : "未配置";
    const url = checkedGatewayOrigin(data.base_url || "http://127.0.0.1:18900");
    $("gateway-console").href = url + "/";
    renderAccounts();
    return data;
  }
  async function connectGateway() {
    message("gateway-error", "");
    message("gateway-message", "");
    const button = $("gateway-connect");
    button.disabled = true;
    try {
      const url = checkedGatewayOrigin($("gateway-url").value.trim());
      const result = await gatewayRequest("connect", {
        base_url: url, forward_key: $("gateway-key").value.trim()
      });
      $("gateway-key").value = "";
      message("gateway-message", result.message + "（" + result.model_count + " 个账号渠道模型）");
      if (!result.host_auth_ready) message("gateway-error", "CLIProxyAPI 的插件 auth 标记保存尚未确认，请参考 README 或 Issue #1 排查。");
      await refreshGateway();
    } catch (err) { message("gateway-error", err.message); }
    finally { button.disabled = false; }
  }
  async function disconnectGateway() {
    if (!confirm("确认断开账号网关并移除本插件保存的 Forward Key？原服务内账号不会退出登录。")) return;
    try {
      const res = await gatewayRequest("disconnect", {});
      $("gateway-key").value = "";
      message("gateway-message", res.message);
      await refreshGateway();
    } catch (err) { message("gateway-error", err.message); }
  }

  function init() {
    renderAccounts();
    if (!secure) {
      $("management-key").disabled = true;
      message("credentials-error", "此地址未使用 HTTPS。请从本机 localhost 打开，或为远程管理启用 HTTPS。");
    }
    $("credentials-form").addEventListener("submit", async (ev) => {
      ev.preventDefault();
      model.key = $("management-key").value.trim();
      if (!model.key) return;
      try { await refresh(); $("credentials").close(); $("management-key").value = ""; }
      catch (err) { message("credentials-error", err.message); }
    });
    $("change-key").addEventListener("click", () => showLogin(""));
    $("search").addEventListener("input", (event) => { model.query = event.target.value; renderCards(); });
    document.querySelectorAll("[data-filter]").forEach((button) => {
      button.addEventListener("click", () => {
        model.filter = button.dataset.filter;
        document.querySelectorAll("[data-filter]").forEach((b) => b.classList.toggle("selected", b === button));
        renderCards();
      });
    });
    document.querySelectorAll("[data-tab]").forEach((button) => {
      button.addEventListener("click", () => {
        const isAccount = button.dataset.tab === "accounts";
        $("provider-section").hidden = isAccount;
        $("account-section").hidden = !isAccount;
        document.querySelectorAll("[data-tab]").forEach((b) => b.classList.toggle("is-active", b === button));
      });
    });
    $("close-editor").addEventListener("click", () => {
      $("provider-key").value = "";
      $("editor").close();
    });
    $("editor-form").addEventListener("submit", (ev) => {ev.preventDefault(); void saveEditor();});
    $("test-connection").addEventListener("click", () => void testConnection());
    $("disconnect").addEventListener("click", () => void disconnect());
    $("gateway-connect").addEventListener("click", () => void connectGateway());
    $("gateway-refresh").addEventListener("click", () => void connectGateway());
    $("gateway-disconnect").addEventListener("click", () => void disconnectGateway());
    $("gateway-console").addEventListener("click", (event) => openAccountConsole(event));
    $("credentials").addEventListener("cancel", (ev) => {
      if (!model.key) ev.preventDefault();
    });
    $("editor").addEventListener("close", () => { $("provider-key").value = ""; });
    showLogin("");
  }
  document.addEventListener("DOMContentLoaded", init);
})();
