# 服务器账号直接授权 / 回调机制核实

研究基线：Ebony-Vinyl/dsh-our-free-model 的 `vendor/channel-pack/src/channel-pack-rpc.ts`、
`packages/standalone/management.mjs`、
`vendor/channel-pack/src/*-oauth.ts`；以仓库具体版本为准。

## 当前真正可走通的 API 交互（服务端开发实现，尚未真实账号验收）

```text
用户点击「添加 Qoder / Qoder CN / Cline / MiniMax Code 账号」
     │ 1. 浏览器同步打开占位授权窗口（防止异步弹窗拦截）
     ▼
CLIProxyAPI /v0/management/ofm-provider/account-gateway/login/start
     │ 2. Management Key 鉴权
     ▼
Go 插件: accountServerRPC("account.create", {provider})
     │ 3. Forward Key 仅在服务器 -> /api/management/channels/rpc
     ▼
Our Free Model Standalone Channel Pack (服务端）
     │ 4. 设备码 / PKCE / 供应商认证 → 返回 loginUrl、accountId
     ▼
用户浏览器跳转供应商 HTTPS 授权页面
     │ 5. 用户登录并确认授权；token 由服务端账号运行时保存
     ▼
Go 插件 login/poll → Channel Pack login.poll
     │ 6. 浏览器每 4 秒自动查询结果；后台真实检查账号凭据
     ▼
已授权 → 插件自动拉取 /v1/models / 保存模型目录
     │ 7. 更新账号池状态和管理界面
     ▼
CLIProxyAPI 模型列表（可能需要重载宿主）→ /v1/chat/completions
```

**不再需要人工查看设备码登录是否成功，也不需要人工点击同步按钮作为完成授权的必要步骤。** 授权成功状态仍只能以 `login.poll` 确认；注册了一个账号或读取到某个模型都不等于其真实可调用。

### 接口对应

| 我们的插件 | DSH Standalone | 内容 |
| --- | --- | --- |
| `/login/start` | `channels/rpc account.create` | 请求账号渠道的正式授权 URL 和账号 ID |
| `/login/poll` | `channels/rpc login.poll` | 凭据写入后返回 `done:true, success:true` |
| `/accounts` | `channels/rpc account.list` | 脱敏显示真实账号池 |
| `/channels/status` | `channels/rpc provider.status` | 真实账号登记数量与模型数（不是可用性测试） |
| `/connect` | `GET /v1/models` | 自动同步成功授权账号的模型缓存 |
| `executor.execute(_stream)` | `POST /v1/chat/completions` | 服务器转发（完整宿主联调待办） |

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
- [ ] 至少一个真实 Qoder 或 MiniMax 账号的 login.start → poll → model.sync → chat + SSE。
- [ ] 完善 Cline 在服务器上缺少 `userCode` 的 fallback 场景。
- [ ] 将服务器端设备码授权结果与 host model registrar 的动态更新机制核实；若不可动态更新，明确提示重载。
- [ ] 解决其余 loopback OAuth/扫码渠道真正的公网 HTTPS 回调能力，逐家验证上游授权条款和官方 OAuth 注册选项。

CI 只覆盖 Go、Python、Node 语法、Linux c-shared ABI 编译；不能代表真实授权成功。
