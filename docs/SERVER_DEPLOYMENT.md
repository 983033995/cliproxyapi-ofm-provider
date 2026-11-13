# 服务端部署方案（账号渠道优先）

本项目仅面向 **CLIProxyAPI 服务器端**。不接入 Ollama、本机模型，也不要求用户在办公电脑或家用电脑上运行任何渠道服务。

## 服务器组件

```text
远程浏览器（HTTPS）
    │ CLIProxyAPI Management Key
    ▼
CLIProxyAPI v8 服务 + ofm-provider.so
    ├── 受管理接口保护的账号列表 / Device Code 登录 / 登录状态 / 模型同步
    └── 服务端内部 HTTP（仅服务器内部） → OFM Account Runtime :18900
                                               ├── 账户凭据、积分、续期
                                               ├── 模型列表及路由
                                               └── 供应商授权服务
```

**重点：**这里的 `127.0.0.1` 指 **CLIProxyAPI 服务器自己的网络命名空间**，不是浏览器所在的计算机，不需要客户端电脑上运行任何进程。账号运行时是服务器侧依赖服务，不是本地大语言模型。

## 服务端组件部署

需要 Linux 服务器、Node.js ^22.19 或 >=24、支持 C ABI 插件的 CLIProxyAPI v8，且管理入口由 HTTPS 反向代理保护。参考 [原项目独立服务说明](https://github.com/Ebony-Vinyl/dsh-our-free-model/blob/main/packages/standalone/README.md)。

服务器终端执行（在确认来源及授权条款后）：

```sh
git clone https://github.com/Ebony-Vinyl/dsh-our-free-model.git /opt/dsh-our-free-model
cd /opt/dsh-our-free-model
git checkout 9aa84122
npm ci --ignore-scripts
npm run start:standalone -- --port 18900 --data-dir /var/lib/ofm-accounts
```

独立服务默认为仅绑定服务器的回环地址。发行物并未完全独立发布，上述安装仍依赖源仓库的资源完整性及系统权限；以其完整文档为准。生产部署使用 `deploy/systemd/ofm-account-runtime.service` 配合专用服务用户和持久化存储目录。建议保留固定的模型缓存及账号数据，做好备份和权限控制。

CLIProxyAPI 服务器环境示例（见 `deploy/server.env.example`）：

```sh
OFM_ACCOUNT_GATEWAY_URL=http://127.0.0.1:18900
OFM_ACCOUNT_FORWARD_KEY=<从服务器账号运行时 settings.json.forwardKey 获取>
OFM_PROVIDERS_FILE=/var/lib/cliproxyapi/ofm-providers.json
OFM_SECRETS_FILE=/var/lib/cliproxyapi/ofm.secrets.json
```

密钥只能写在服务端受保护的环境/机密文件中；不提交仓库、不发到浏览器。CLIProxyAPI 对外提供常规推理 API 和经过反向代理 HTTPS 保护的插件管理资源；**账号服务端口 18900 不对公网开放**。启动后可以在管理页面「账号授权渠道」检查服务状态和同步模型。

## 远程登录渠道

当前插件新建管理 API，通过 CLIProxyAPI Management Key 鉴权，服务器端再用 Forward Key 调用账号运行时的 `POST /api/management/channels/rpc`。前端从不直接向 `127.0.0.1:18900` 发送请求。

- 第一批 **可发起远程 Device Code 授权**：Qoder、Qoder 中国版（内部 ID `qodercn`）、Cline、MiniMax Code。
- 接口：`POST /v0/management/ofm-provider/account-gateway/login/start`（返回远程 HTTPS 授权链接）与 `login/poll`（查看状态），`accounts` 用于读取脱敏后的账号信息。
- 有回环 OAuth 回调 / 微信扫码载体 / 本地浏览器依赖的渠道 **不得直接返回服务器 localhost 登录链接给远程浏览器**；目前属于服务端待适配（如 TRAE、Gemini、LobsterAI、Loomy、Raccoon）。需要额外设计合规的远程回调或设备码流程。
- ZCode 是否需要服务器页面/验证码支持尚未验证，保持为待适配；已经在账号运行时的已授权账号可同步其模型。
- 授权完成以后点击「重新同步」，再重载 CLIProxyAPI 插件，使其静态模型目录更新；是否确实可以推理，需要真实宿主集成测试。

**重要限制：**上游渠道大量依赖逆向的第三方桌面客户端协议，其账户合同可能限制自动化、转发或服务器用途。提供服务器端管理接入接口不等于确认十家供应商授权远程代理调用。应逐家验证并遵守其服务条款，不规避账户或区域限制。

## Docker / 容器注意事项

原账号运行时 **只监听 loopback**；容器的 localhost 不是宿主机器 localhost。若使用容器，需要将 CLIProxyAPI 和账号运行时放在**同一个网络命名空间**（例如同一个 Pod；Linux 上也可通过 `network_mode: service:<cli-proxy-service>` 或经网络隔离评审后的 host networking）。不可简单改为监听 `0.0.0.0` 暴露登录或凭据服务，也不应把整个管理端口暴露到互联网。

有正式容器版/OCI 镜像之前，推荐先通过服务器 systemd 运行账号服务及 CLIProxyAPI 进程，完成鉴权与 API 流式联调。

## 验收门槛

- [ ] CLIProxyAPI v8 真实宿主插件加载、Management Key 鉴权、auth marker 路由
- [ ] 服务器同网络命名空间内能够到达账号运行时 `/v1/models`
- [ ] 远程 HTTPS 管理页能为 Device Code 渠道发起授权，取得真实账号状态
- [ ] 账号令牌不经过浏览器/CLIProxyAPI auth marker；安全审查管理端点与日志
- [ ] 新授权账号模型出现在 CLIProxyAPI `/v1/models`
- [ ] 至少一个被正式授权的模型通过普通 /v1/chat/completions 与 SSE 完整端到端推理
- [ ] 401/429/5xx、流中断、取消、配额耗尽与失败清理
- [ ] 服务重启、凭据权限、模型同步、灰度回滚

现在的 CI 仅覆盖单元测试、前端语法和 c-shared 编译。**未完成上述验收前不宣称生产可用。**
