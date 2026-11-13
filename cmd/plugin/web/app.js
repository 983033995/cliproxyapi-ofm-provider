"use strict";

(function () {
  const $ = (id) => document.getElementById(id);
  const path = "/v0/management/ofm-provider/providers";
  const model = {key: "", providers: [], filter: "all", query: "", tab: "accounts", selected: null, gateway: null, pendingLogin: null, channelStatuses: {}};
  let authPollTimer = null;
  let authPollInFlight = false;
  const secure = location.protocol === "https:" || ["127.0.0.1", "localhost", "::1", "[::1]"].includes(location.hostname);
  const accountChannels = [
    ["LobsterAI", "有道", "回环 OAuth 授权，需要服务器端回调适配"],
    ["Qoder", "阿里系", "设备码授权 · 登录结果由服务器自动确认"],
    ["Qoder 中国版", "阿里系", "设备码授权 · 支持远程浏览器登录"],
    ["TRAE", "字节", "固定回环地址 OAuth 回调，远程适配待完成"],
    ["Cline", "Cline", "设备码授权 · 支持远程浏览器登录"],
    ["Loomy", "讯飞", "微信扫码登录需要服务端二维码载体适配"],
    ["Raccoon", "商汤", "微信扫码及短信登录需要服务端回调适配"],
    ["MiniMax Code", "MiniMax", "OAuth 设备码及 PKCE · 支持远程浏览器授权"],
    ["ZCode", "智谱", "浏览器及验证码交互待服务器适配"],
    ["Gemini Code Assist", "Google", "原生应用回环 OAuth 回调，远程适配待完成"]
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
    stopAuthPolling();
    model.pendingLogin = null;
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
    $("total").textContent = "10";
    $("active").textContent = model.gateway?.enabled ? "正常" : "未连接";
    $("available").textContent = String(model.gateway?.connected_models || 0);
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
    if (planned) {
      const activeAccounts = p.accountStatus?.accounts_enabled ?? 0;
      const registered = p.accountStatus?.accounts_total ?? 0;
      status[0] = activeAccounts ? "已登记 " + activeAccounts + " 账号" : registered ? "账号已禁用" : "未授权";
      status[1] = activeAccounts ? "tag live" : "tag";
    }
    const tag = el("span", status[1], status[0]);
    top.append(ident, tag);

    const desc = el("p", "card-description", planned ?
      p.description : label(p.auth_kind) + " · " + (p.format || "未知协议") + " · " + (p.base_url || "未设置接入地址"));
    const metrics = el("div", "card-metrics");
    const modelCount = el("span", "", "模型 ");
    modelCount.append(el("b", "", String((p.models || []).length)));
    metrics.append(modelCount);
    if (planned) {
      const accounts = el("span", "", "账号 ");
      const count = Number(p.accountStatus?.accounts_total ?? 0);
      accounts.append(el("b", "", String(count)));
      metrics.append(accounts);
    }
    if (!planned) {
      const key = el("span", "", "凭据 ");
      key.append(el("b", p.credential_present ? "ok" : "", p.auth_kind === "none" ? "不需要" :
        (p.credential_present ? "已配置" : "未配置")));
      metrics.append(key);
    }
    const actions = el("div", "card-actions");
    const button = el("button", p.supported && !planned ? "accent" : "", planned ? (p.remoteLogin ? "添加账号" : "查看账号") : p.supported ?
      (p.enabled ? "管理接入" : "接入设置") : "待适配");
    button.type = "button";
    if (planned) button.addEventListener("click", () => p.remoteLogin ? void beginServerLogin(p.providerID,p.name) : void showServerAccounts(p.providerID,p.name));
    else if (p.supported) button.addEventListener("click", () => openEditor(p.id));
    else {
      button.disabled = true;
      button.title = "需要独立的协议或账号授权适配器，暂不能使用";
    }
    const tip = el("small", "", planned ? (p.remoteLogin ? "支持远程设备码授权" : "远程登录待适配") :
      (p.credential_source === "local-file" ? "本地私有凭据文件" :
        (p.credential_source === "environment" ? "环境变量凭据" :
          (p.supported ? "启用后需要模型目录重载" : "需额外协议适配"))));
    actions.append(button, tip);
    if (planned && p.remoteLogin) {
      const view = el("button", "", "查看账号");
      view.type = "button";
      view.addEventListener("click", () => void showServerAccounts(p.providerID,p.name));
      actions.append(view);
    }
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
      ["lobsterai"], ["qoder"], ["qodercn", "qoder-cn", "qoder_cn"], ["trae"],
      ["cline"], ["loomy"], ["raccoon"], ["minimax", "minimax-code"], ["zcode"], ["gemini"]
    ];
    const synced = model.gateway?.models || [];
    const remoteLoginIds = new Set(["qoder","qodercn","cline","minimax"]);
    accountChannels.forEach((a, index) => {
      const models = synced.filter((m) => prefixes[index].includes(String(m.id || "").split("/")[0].toLowerCase()));
      const providerID = prefixes[index][0];
      const accountStatus = model.channelStatuses?.[providerID] || null;
      box.append(card({
        id: "account-" + index, providerID, accountStatus,
        remoteLogin: remoteLoginIds.has(providerID),
        name: a[0], owner: a[1], description: a[2],
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
    $("active").textContent = data.enabled ? "正常" : "未连接";
    $("available").textContent = String(data.connected_models || 0);
    renderAccounts();
    try { await refreshChannelStatus(); }
    catch (err) { message("gateway-error", "账号池状态暂不可读取：" + err.message); }
    return data;
  }
  async function refreshChannelStatus() {
    const result = await gatewayRequest("channels/status");
    model.channelStatuses = result.statuses || {};
    renderAccounts();
    return result;
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

  async function hostOAuthRequest(path) {
    if (!secure) throw Error("远程 OAuth 管理要求 HTTPS");
    const res = await fetch("/v8/management/oauth/" + path, {
      method:"GET", headers:{"X-Management-Key":model.key,"Accept":"application/json"},
      cache:"no-store",credentials:"same-origin",redirect:"error"
    });
    if (res.status === 401 || res.status === 403) {
      showLogin("CLIProxyAPI OAuth 管理接口需要重新认证");
      throw Error("CLIProxyAPI 原生 OAuth 管理接口未授权");
    }
    let payload;
    try { payload = await res.json(); } catch (_) { throw Error("CLIProxyAPI OAuth 接口响应不是 JSON"); }
    if (!res.ok || payload.status === "error") {
      throw Error(payload.error || "原生 OAuth 接口不可用，HTTP " + res.status);
    }
    return payload;
  }
  async function startHostOAuth(provider) {
    const id = "ofm-" + provider;
    const result = await hostOAuthRequest("auth-url?provider=" + encodeURIComponent(id) + "&is_webui=true");
    if (result.status !== "ok" || !result.url || !result.state)
      throw Error("CLIProxyAPI 未返回有效的授权链接与登录会话状态");
    return {login_url:result.url, oauth_state:result.state, native:true};
  }
  async function pollHostOAuth(state) {
    const result = await hostOAuthRequest("status?state=" + encodeURIComponent(state));
    if (result.status === "wait" || result.status === "pending")
      return {state:"pending",message:"等待账号完成登录；由 CLIProxyAPI 正在轮询原生授权状态"};
    if (result.status === "ok")
      return {state:"authorized",message:"CLIProxyAPI 已保存授权记录",native:true};
    if (result.status === "error")
      return {state:"failed",message:result.error || "原生 OAuth 授权失败"};
    return {state:"pending",message:"等待 CLIProxyAPI 更新登录状态"};
  }

  function stopAuthPolling() {
    if (authPollTimer !== null) clearTimeout(authPollTimer);
    authPollTimer = null;
  }
  function scheduleAuthPoll(delay = 4000) {
    stopAuthPolling();
    if (!model.pendingLogin) return;
    authPollTimer = setTimeout(() => { void pollServerLogin(); }, delay);
  }
  function secureAuthorizationLink(url) {
    const parsed = new URL(url);
    if (parsed.protocol !== "https:" || parsed.username || parsed.password ||
        parsed.hash || !parsed.hostname ||
        ["localhost","127.0.0.1","::1","[::1]"].includes(parsed.hostname.toLowerCase()) ||
        parsed.hostname.endsWith(".local")) throw Error("授权服务没有返回可远程打开的安全 HTTPS 地址");
    return parsed.href;
  }
  async function beginServerLogin(providerID, providerName) {
    stopAuthPolling();
    model.pendingLogin = null;
    $("account-auth-panel").hidden = false;
    $("account-auth-heading").textContent = "添加 " + providerName + " 账号";
    $("account-auth-link").hidden = true;
    $("account-login-poll").disabled = true;
    $("account-auth-refresh").disabled = false;
    message("gateway-error", "");
    message("account-auth-message", "正在向服务器请求正式授权链接…");

    // Open synchronously on the click gesture to avoid browser popup blockers.
    // If it is blocked, the explicit link below remains the reliable fallback.
    let popup = null;
    if (model.gateway?.credential_present) {
      popup = window.open("about:blank", "_blank");
      if (popup) {
        try {
          popup.opener = null;
          popup.document.title = "正在打开账号授权…";
          popup.document.body.textContent = "正在向服务器请求账号授权链接，请在此页面完成授权。";
        } catch (_) { /* user/browser may close the popup */ }
      }
    }
    try {
      // Use CLIProxyAPI v8's built-in OAuth login lifecycle and callback/session
      // persistence, not an isolated fake "authorization" button.
      const response = await startHostOAuth(providerID);
      const href = secureAuthorizationLink(response.login_url);
      model.pendingLogin = {
        provider: providerID, name: providerName, oauthState:response.oauth_state,
        native:true, deadline:Date.now()+9*60*1000,errors:0
      };
      $("account-auth-link").href = href;
      $("account-auth-link").hidden = false;
      $("account-login-poll").disabled = false;
      if (popup && !popup.closed) {
        try { popup.location.replace(href); }
        catch (_) { /* link fallback is shown below */ }
      }
      message("account-auth-message",
        "授权窗口已打开。完成登录后系统每 4 秒自动检查回调结果，无需手动复制 Token。若弹窗被拦截，请点击下方授权链接。");
      void showServerAccounts(providerID, providerName, true);
      scheduleAuthPoll(4000);
    } catch (err) {
      if (popup && !popup.closed) { try { popup.close(); } catch (_) {} }
      message("account-auth-message", "无法发起授权：" + err.message);
    }
  }

  async function pollServerLogin() {
    const pending = model.pendingLogin;
    if (!pending || authPollInFlight) return;
    if (Date.now() > pending.deadline) {
      stopAuthPolling();
      model.pendingLogin = null;
      $("account-login-poll").disabled = true;
      message("account-auth-message", "授权等待已超时，请重新点击「添加账号」发起新的授权。");
      return;
    }
    authPollInFlight = true;
    try {
      const state = pending.native ? await pollHostOAuth(pending.oauthState) :
        await gatewayRequest("login/poll", {account_id: pending.accountID});
      if (model.pendingLogin !== pending) return; // another login superseded this one
      pending.errors = 0;
      if (state.state === "authorized") {
        stopAuthPolling();
        model.pendingLogin = null;
        $("account-auth-link").hidden = true;
        $("account-login-poll").disabled = true;
        message("account-auth-message", "账号授权成功，服务器已收到回调并保存账号。");
        await showServerAccounts(pending.provider, pending.name, true);
        await refreshGateway();
        if (state.native) {
          message("gateway-message",
            "CLIProxyAPI 原生 OAuth 已确认登录并保存认证记录。账号模型目录已尝试自动刷新；若新模型未出现，请重载宿主或检查账号运行时。");
        } else if (state.model_sync_ok) {
          message("gateway-message",
            "账号已自动同步 " + state.synced_models + " 个模型；CLIProxyAPI 如未显示，请重载插件或宿主。");
        } else {
          message("gateway-error", "账号授权成功但模型同步失败，请检查服务器账号运行时。");
        }
        return;
      }
      if (state.state === "failed") {
        stopAuthPolling();
        model.pendingLogin = null;
        $("account-login-poll").disabled = true;
        message("account-auth-message", state.message || "授权失败或已经过期，请重新登录。");
        return;
      }
      message("account-auth-message", "等待 " + pending.name + " 完成授权回调… 服务器正在自动检查。");
    } catch (err) {
      if (model.pendingLogin !== pending) return;
      pending.errors += 1;
      message("account-auth-message", "检查授权状态遇到问题：" + err.message);
      if (pending.errors >= 4) {
        stopAuthPolling();
        message("account-auth-message", "连续检查失败，请确认服务器账号运行时正常，或点击「检查授权结果」重试。");
        return;
      }
    } finally {
      authPollInFlight = false;
      if (model.pendingLogin === pending && pending.errors < 4 && authPollTimer === null) scheduleAuthPoll();
    }
  }

  async function showServerAccounts(providerID,providerName,keepMessage=false) {
    $("account-auth-panel").hidden = false;
    $("account-auth-heading").textContent = providerName + " · 服务器账号";
    $("account-auth-refresh").disabled = false;
    $("account-auth-refresh").onclick = () => void showServerAccounts(providerID,providerName);
    if (!keepMessage) message("account-auth-message","正在从服务器读取账号列表…");
    clear($("account-entries"));
    try {
      const data = await gatewayRequest("accounts", {provider:providerID});
      const entries = Array.isArray(data.accounts) ? data.accounts : [];
      if (!keepMessage) message("account-auth-message", "服务器中登记账号 " + entries.length + " 个；登记不表示当前额度仍有效。");
      entries.forEach((a) => {
        const row = el("div","account-entry");
        const left = el("span","",a.nickname || a.id || "未知账号");
        const state = el("span","",a.enabled ? "已启用 · 待用量验证" : "已禁用");
        row.append(left,state);
        $("account-entries").append(row);
      });
      if (!entries.length) $("account-entries").append(el("p","muted","此渠道尚未登记账号。"));
    } catch (err) { message("account-auth-message", "读取账号失败：" + err.message); }
  }

  function init() {
    renderAccounts();
    if (!secure) {
      $("management-key").disabled = true;
      message("credentials-error", "服务器远程管理必须使用 HTTPS，并正确设置 Management Key。");
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
    $("account-login-poll").addEventListener("click", () => void pollServerLogin());

    $("credentials").addEventListener("cancel", (ev) => {
      if (!model.key) ev.preventDefault();
    });
    $("editor").addEventListener("close", () => { $("provider-key").value = ""; });
    window.addEventListener("pagehide", stopAuthPolling);
    showLogin("");
  }
  document.addEventListener("DOMContentLoaded", init);
})();
