# CLIProxyAPI OFM Account Provider

> **服务器端 CLIProxyAPI v8 账号渠道插件 · 开发版**。优先接入 [Our Free Model](https://github.com/Ebony-Vinyl/dsh-our-free-model) 的 Qoder、Cline、MiniMax Code、TRAE、LobsterAI、Loomy、Raccoon、ZCode、Gemini 等账号渠道。目标是通过 CLIProxyAPI 管理页面完成服务器账号授权，并让已经授权的模型在 CLIProxyAPI 标准 API 中可用。

**不以本地大模型 / Ollama / 个人电脑服务为目标。** 项目仍保留之前的 API Key Provider 兼容代码，但不是当前开发优先级。

## 服务端架构

```text
远程管理浏览器 (HTTPS)
          │ CLIProxyAPI Management Key
          ▼
  CLIProxyAPI v8 + ofm-provider.so
        │   ├─ 账号管理页面 / Device Code 授权 / 模型同步
        │   └─ /v1/chat/completions、SSE
        ▼
  服务器内部 OFM Account Runtime (Node.js，默认端口 18900)
        │   ├─ 账号凭据、续期、额度
        │   ├─ 模型发现与执行
        ▼
  官方授权账号对应的供应商
```

账号服务运行在**服务器**，与 CLIProxyAPI 共享网络命名空间。它的服务器内部回环地址不是用户电脑的 localhost；**不应直接开放账号服务的管理端口到公网**。

目前优先以原项目已经提供的 [Standalone 账号运行时](https://github.com/Ebony-Vinyl/dsh-our-free-model/blob/main/packages/standalone/README.md) 作为受控服务器依赖，插件负责账号管理网关、模型注册、请求转发；后续再考虑服务器端托管式 SidecarManager 或获得正式协议许可后的原生渠道适配。此版本不是不依赖 Node 运行时的单文件插件。

## 直接发起 CLIProxyAPI 原生账号授权（当前开发分支）

**重点更新：** 不再将「添加账号」按钮当作仅展示的设置项。管理页对支持 Device Code 的渠道，直接向 CLIProxyAPI v8 的原生 OAuth 接口申请登录链接：

```text
/v8/management/oauth/auth-url?provider=ofm-qoder&is_webui=true
/v8/management/oauth/status?state=<上述接口返回的 state>
```

用户点击「添加账号」时会打开真实第三方 HTTPS 授权窗口，浏览器自动轮询 CLIProxyAPI；授权确认后，插件调用 DSH Account Runtime 的 `login.poll`，并让 CLIProxyAPI 将独立、无第三方 Token 的账号认证标记写入自身凭据目录。同时自动尝试刷新模型目录（宿主可能需要重新加载模型注册）。

当前原生 OAuth 入口：`ofm-qoder`、`ofm-qodercn`、`ofm-cline`、`ofm-minimax`。目标 CLIProxyAPI 需要支持 **Plugin RPC Schema v10 / MultiAuthProvider**。其他需要固定 `127.0.0.1` OAuth 回调或扫码载体的渠道，尚未实现服务器远程登录，不能假装一键完成。

**仍是 Draft POC：CI 编译通过并不代表已经用真实账号完成 OAuth / 推理。** 对照 [授权协议和服务器回调能力矩阵](docs/AUTH_CALLBACK_MATRIX.md)。

## 账号远程授权能力

| 账号渠道 | 服务器远程授权入口 | 说明 |
| --- | --- | --- |
| Qoder、Qoder 中国版 | Device Code 原型 | 已编写 `account.create / login.poll` 桥接；真实账号未验收 |
| Cline | Device Code 原型 | 部分账号授权还可能需要 user_code；待实测 |
| MiniMax Code | Device Code + PKCE 原型 | 真实账号未验收 |
| TRAE、LobsterAI、Gemini | 待适配 | 上游登录依赖浏览器回调到 127.0.0.1，不能直接远程打开 |
| Loomy、Raccoon | 待适配 | 上游扫码/短信登录页由本地回环服务器承载 |
| ZCode | 待适配 | 需复核验证码/载体与服务器浏览器能力 |

已有账号服务中的模型可以经 `/v1/models` 发现并同步（仍须真实宿主验收），即使该渠道的**新增远程登录**暂未适配。已同步 ≠ 登录有效或额度可用。

## 部署与配置

- [服务器专用部署说明、systemd、网络与授权注意事项](docs/SERVER_DEPLOYMENT.md)
- [Account Bridge 早期 POC 文档](docs/ACCOUNT_CHANNEL_BRIDGE.md)
- [CLIProxyAPI 插件基本配置](examples/cliproxyapi-config.v8.yaml)
- [开发路线图](ROADMAP.md)

服务器环境配置：

```sh
OFM_ACCOUNT_GATEWAY_URL=http://127.0.0.1:18900
OFM_ACCOUNT_FORWARD_KEY=SET_AS_SERVER_SECRET
OFM_PROVIDERS_FILE=/var/lib/cliproxyapi/ofm-providers.json
OFM_SECRETS_FILE=/var/lib/cliproxyapi/ofm.secrets.json
```

其中 Forward Key 是**原服务器账号运行时**用于 /v1/models 和推理 API 的密钥；CLIProxyAPI 终端用户调用模型时使用 **CLIProxyAPI 自己的 API Key**。密钥必须保存在服务器受保护的秘密文件/环境变量中，不允许提交仓库。

编译及单元检查（Linux AMD64 开发版）：

```sh
go test ./...
python3 -m unittest discover -s tests -v
node --check cmd/plugin/web/app.js
CGO_ENABLED=1 go build -buildmode=c-shared -o ofm-provider.so ./cmd/plugin
```

将 `ofm-provider.so` 放入 CLIProxyAPI v8 的插件目录并启用。通过**HTTPS 反向代理**远程访问：

```text
https://YOUR-CLIPROXY-HOST/v0/resource/plugins/ofm-provider/dashboard
```

输入 CLIProxyAPI 的 Management Key，然后在**服务器账号授权渠道**选择供应商登录，或连接/同步服务器内部账号运行时。

## 仍待验收

- CLIProxyAPI v8 真正加载 ABI，正确选择 `ofm-provider` auth，并将账号模型注册到宿主模型清单。
- 远程 Device Code 授权是否被供应商实际接受；不尝试绕过上游客户端或账户限制。
- 经 /v1/chat/completions 和 SSE 实际生成，计量和出错恢复。
- 多账号状态及额度查询、换号、取消/流中断、重启/回滚。
- Docker 网络命名空间、systemd 运行权限、远程 HTTPS 管理安全。

CI 仅代表 **Go 单元测试、前端语法和本机 ABI 编译检查**，不能替代服务器真实调用验证。PR 仍为 Draft，不应标记生产发布。

## 来源与许可

- 原账号渠道包：Jet（MIT），在 dsh `vendor/channel-pack/LICENSE` 和 `NOTICE.md` 标明出处；本仓库目前未复制其执行包或 WASM。
- dsh 研究基线：`9aa84122`；CLIProxyAPI v8 研究基线：`1aaf56bc`，宿主版本/ABI 可能更新，必须与实际部署镜像对照。
- 使用各家账号需遵守适用服务条款、官方授权范围与额度限制，不将「有登陆」等同「可获授权代理调用」。
