# 服务器账号直接授权 / 回调机制核实

研究基线：Ebony-Vinyl/dsh-our-free-model 的 `vendor/channel-pack/src/channel-pack-rpc.ts`、
`packages/standalone/management.mjs`、
`vendor/channel-pack/src/*-oauth.ts`；以仓库具体版本为准。

## 新流程：直接使用 CLIProxyAPI v8 原生 OAuth（尚未真实账号验收）

这次不再只使用插件的自定义 `/v0/management/.../login/start`。目标宿主需要支持 **plugin RPC Schema v10**（多 OAuth Provider）；当前插件 `auth.identifier` 提供以下可操作授权入口：

`ofm-qoder`、`ofm-qodercn`、`ofm-cline`、`ofm-minimax`。

```text
浏览器点击「添加 Qoder 账号」
    │ 预打开新标签避免异步弹窗被拦截
    ▼
GET /v8/management/oauth/auth-url?provider=ofm-qoder&is_webui=true
    │ CLIProxyAPI 验证 Management Key，执行插件 auth.login.start
    ▼
插件向服务器 OFM Account Runtime 发送 channel-pack account.create
    │ 获取账户ID + 供应商 HTTPS 授权地址
    ▼
CLIProxyAPI 注册 OAuth session (随机 State, 到期时间)
    │ 浏览器立即跳到真实供应商授权页面
    ▼
用户点击确认 / 登录 → 服务端运行时保管上游凭据
    │ 原页面每 4 秒请求 CLIProxyAPI /v8/management/oauth/status?state=...
    ▼
CLIProxyAPI 调插件 auth.login.poll → 运行时 channel-pack login.poll
    │ 授权成功，插件返回独立的 AuthData / StorageJSON 认证标记
    ▼
CLIProxyAPI 保存账号认证文件 + 插件自动同步 /v1/models
    │ 新模型可能需要宿主重新执行 model.register
    ▼
CLIProxyAPI 的 /v1/models 和 /v1/chat/completions
```

**第三方账号 token 仍只由服务器 OFM Account Runtime 保管。** CLIProxyAPI 持久化的是无第三方 token 的认证标记、账号 ID 与 OAuth 渠道信息。登录失败、过期、网络异常均应显示真实状态而不是虚构「已接入」。

### 接口与代码对应

| CLIProxyAPI 原生接口/插件方法 | OFM Standalone 内部调用 | 作用 |
| --- | --- | --- |
| `GET /v8/management/oauth/auth-url?provider=ofm-qoder` → `auth.login.start` | `channels/rpc: account.create` | 获取官方设备码/PKCE 授权 URL、创建 CLIProxyAPI session |
| `GET /v8/management/oauth/status?state=...` → `auth.login.poll` | `channels/rpc: login.poll` | 自动检查授权状态，成功后让 CLIProxyAPI 保存 AuthData |
| `auth.identifier` + Schema v10 | — | 注册四个可用的命名 OAuth 入口及显示名称 |
| `auth.parse` / `auth.refresh` | — | 恢复服务器上的独立账号认证标记，保持唯一 ID |
| `GET /v0/management/ofm-provider/account-gateway/channels/status` | `channels/rpc: provider.status` | 显示真实登记账号数量，不能替代推理可用性 |
| `POST /v0/management/ofm-provider/account-gateway/accounts` | `channels/rpc: account.list` | 只展示脱敏账号信息 |
| `model.register` / `executor.execute_stream` | `GET /v1/models` / `POST /v1/chat/completions` | 注册模型及服务器内流式转发（真实宿主仍待联调） |

```sh
# CLIProxyAPI v8 统一 OAuth 测试入口（要求 Management Key）
curl -H "X-Management-Key: YOUR_MANAGEMENT_KEY" \
  "https://YOUR-CLIPROXY-HOST/v8/management/oauth/auth-url?provider=ofm-qoder&is_webui=true"
```

正常宿主应返回 `status:ok`、`url`、`state`；然后在浏览器打开 `url` 并轮询 `status?state=...`。**此处是接口预期，不是已在真实服务器上验证的成功样例。**

### 账号渠道为什么不能统一一个 OAuth 回调实现？

源代码的协议存在真实差异，不能伪造接入状态：

| 渠道 | 源码认证方式 | 服务器远程网页接入 |
| --- | --- | --- |
| Qoder / Qoder 中国版 | Device Code（本机不需要固定回调监听） | 已实现「授权页面→自动轮询」原型 |
| Cline | WorkOS Device Code，可选 `user_code` | 已实现授权链接与轮询原型，`user_code` 需完善 |
| MiniMax Code | Device Code + PKCE | 已实现授权链接与轮询原型 |
| TRAE | 固定 `127.0.0.1:18080/authorize` OAuth 回调 | **不支持直接远程回调**；需独立 HTTPS 回调适配/厂商允许的回调注册 |
| LobsterAI | `127.0.0.1:随机端口/auth/callback` | **不支持直接远程回调**，需重新设计受允许的回调方式 |
| Gemini Code Assist | Google 原生应用 `localhost:<port>/oauth-callback` | **不支持直接远程回调**，需正式 Web OAuth Client + 注册 HTTPS Redirect URI |
| Loomy、Raccoon | 微信/短信认证与本机生成的二维码载体页 | **需要安全的服务器端二维码展示与回调适配** |
| ZCode | 登录包含浏览器、验证码/短信及账号设备流程 | **需要单独适配并实测** |

不能只对原作者返回的 `http://127.0.0.1` 授权链接改字符串为服务器域名：
供应商回调地址、OAuth state、PKCE 验证器、授权客户端注册配置和原运行时的本机监听方式必须**同时匹配**。否则即使用户在浏览器登录成功，也无法把令牌回传服务器。尤其不能在公网暴露原账号服务的登录端口来绕过验证。

### 安全边界

- 浏览器只连接 CLIProxyAPI HTTPS 管理页面和供应商官方 HTTPS 认证页。
- CLIProxyAPI Management Key 保持管理用途；内部 Forward Key 永远不返回前端。
- 插件端仅转发认证 `loginUrl` / 非敏感账号标识 / 授权完成状态；第三方 token 在服务器账号运行时保存。
- OAuth 授权 URL 验证 HTTPS、非 loopback、无嵌入凭据；异常不展示假的“已登录”。
- 不承诺通过非官方/模拟客户端绕过区域、额度或平台条款约束。

### P0 未完成验证

- [ ] 在真实 Linux CLIProxyAPI v8 与服务器 OFM Standalone 进程上联调。
- [ ] 至少一个真实 Qoder 或 MiniMax 账号经 CLIProxyAPI **v8 原生 OAuth API** 完成 start → poll → AuthData 落盘 → 模型同步 → chat + SSE。
- [ ] 完善 Cline 在服务器上缺少 `userCode` 的 fallback 场景。
- [ ] 将服务器端设备码授权结果与 host model registrar 的动态更新机制核实；若不可动态更新，明确提示重载。
- [ ] 解决其余 loopback OAuth/扫码渠道真正的公网 HTTPS 回调能力，逐家验证上游授权条款和官方 OAuth 注册选项。

CI 只覆盖 Go、Python、Node 语法、Linux c-shared ABI 编译；不能代表真实授权成功。
