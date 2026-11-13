# 服务端账号渠道优先级（2026-11-13）

**第一目标：**在 Linux 服务器上部署 CLIProxyAPI v8 插件，通过受 Management Key 保护的远程 HTTPS 页面授权第三方账号，并在 CLIProxyAPI 标准 API 中调用其模型。个人电脑本地模型、Ollama、个人电脑上的账号服务均不作为项目目标。

- [x] 服务器内部账号运行时桥接、模型同步与 SSE 转发代码
- [x] 远程账号列表与 Qoder/Qoder CN/Cline/MiniMax Device Code 授权接口原型
- [x] 同服务器网络命名空间、systemd 和服务端密钥部署文档
- [ ] 真实 CLIProxyAPI v8 宿主鉴权选择与模型加载（Issue #1）
- [ ] 至少一个合法授权账号完成远程授权及生成、SSE 真实测试（Issue #4）
- [ ] 无本机回环回调依赖的服务端 OAuth / 扫码流程逐家完善
- [ ] Linux 容器部署、证书、反向代理、凭据刷新和额度查询
- [ ] 发布版本、回滚、账号隔离与端到端用量监控

当前旧 Provider/API Key 计划仅保留作次要兼容层，不应优先扩展本地免密 Provider。

---

# Provider Plugin Development Plan

## Goal

One installable `ofm-provider.so` plugin for multiple third-party model vendors from the DSH registry. The plugin owns its provider catalog, model namespaces and upstream execution; CLIProxyAPI is the host, not the project being modified.

## Phase P0 — Repository foundation (POC)
- [x] Independent repository initialized.
- [x] Go C ABI entry point + model registry + executor prototype.
- [x] Safe JSON registry import from DSH with disabled defaults.
- [x] Unit tests and CI build checks.
- [ ] Test this exact remote repository's CI status.

## Phase P1 — Chat/API Key end-to-end
- [ ] Confirm production CLIProxyAPI v8 image supports the schema and capability tuple.
- [ ] Ensure a valid auth candidate exists for the plugin's static model registrations: test empty auths and a plugin auth record.
- [ ] `/v1/models`, non-streaming `/v1/chat/completions`, streaming SSE with actual authorized upstream.
- [ ] Check `executor.execute` and `execute_stream` error envelopes and real host retries.
- [ ] Handle SSE JSON fallback, missing terminal frame, cancellation, concurrent >=8 streams, upstream and downstream closing.
- [ ] Treat model name collisions and provider config reload as explicit validation errors.

## Phase P2 — Additional provider formats
- [ ] Native Anthropic Messages: system, tools, cache usage and SSE event vocabulary.
- [ ] Native OpenAI Responses: input/output items, tool calls, token usage, stream termination.
- [ ] OAuth/ADC only through official authorization mechanisms and approved provider contracts.
- [ ] Provider-specific metadata discovery and credential refresh (where authorized).
- [ ] Runtime model/credential management UI, optional.

## Release gates
- [ ] Tests in Linux/macOS target architectures + libc/CGO ABI check.
- [ ] Security: URL allowlisting, secret redaction, per-provider TLS, timeouts, cancellation and no silent retry on 429.
- [ ] Offline mock-provider integration tests and isolated real CLIProxyAPI.
- [ ] Attribution / LICENSE / NOTICE review before including third-party assets.
- [ ] Canary deployment and documented rollback.

Never claim a provider works solely because it appears in the static JSON catalog.
