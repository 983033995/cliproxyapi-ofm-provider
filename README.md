# CLIProxyAPI OFM Provider

> 一个独立的 CLIProxyAPI v8 多模型供应商插件（开发版）。从 [dsh-our-free-model](https://github.com/Ebony-Vinyl/dsh-our-free-model) 的第三方 Provider Registry 导入供应商目录，通过一个 c-shared 插件完成模型注册与出站请求分发。

## 范围

- **一个插件，多个供应商**：每个 Provider 配置独立的 base_url、API Key 环境变量、模型清单和开关。
- 统一模型命名：`ofm/<provider-id>/<upstream-model-id>`，保留上游模型 ID 中的斜杠。
- 第一期：`openai-chat`，支持用户合法持有 API Key 的供应商及明确配置的本地无 Key 服务。
- 后续独立适配 Anthropic Messages、OpenAI Responses；OAuth/ADC 须使用正式授权流程。
- 不复制 dsh 桌面 UI、本地 18899 网关或模拟官方客户端/规避上游限制的功能。

## 当前进度

- Go c-shared 插件开发工程、Provider 导入器和单元测试：**POC**。
- 正式宿主模型发现、认证选择、非流式 / SSE / 冷却重试：**尚须 CLIProxyAPI v8 真实集成验收**。
- 生产部署：**未就绪**，请勿将 CI 编译通过当作可以生产使用。

## 可视化供应商接入（开发预览）

现在插件通过 CLIProxyAPI v8 的 \`ManagementAPI\` 注册了**供应商配置入口**，无需手动编辑 JSON 来管理 API Key：

1. 在 CLIProxyAPI 的 \`config.yaml\` 中启用 \`plugins.enabled: true\` 与 \`plugins.configs.ofm-provider.enabled: true\`，设置 \`management.secret-key\`；默认仅允许本机管理。
2. 构建 \`ofm-provider.so\`，放入宿主 \`plugins.dir\` 目录，重启 CLIProxyAPI。
3. 在宿主本机浏览器中打开：
   \`http://127.0.0.1:8317/v0/resource/plugins/ofm-provider/dashboard\`
4. 输入 CLIProxyAPI Management Key → 选择 Provider → 「接入设置」→ 输入 API Key → 「测试连接」→ 「保存并启用」。
5. 插件将配置写入本地 \`ofm-providers.json\`，将 Key 写入独立 **0600 私有文件** \`ofm-providers.json.secrets.json\`，不会在管理接口回显 Key。该文件未加密，务必只运行在可信宿主上。
6. **模型目录的变更需要重载插件/重启 CLIProxyAPI 才能确保宿主重新注册模型**。是否真实可调用还必须通过 Issue #1 的宿主鉴权/执行测试。

管理 API 使用 CLIProxyAPI 的 \`Authorization: Bearer <management-key>\` 或 \`X-Management-Key\` 鉴权。插件资源路由 **不鉴权**，所以只返回静态 HTML、CSS、JS；所有列出状态/配置 Key/测试/禁用操作都在 \`/v0/management/ofm-provider/... \` 经宿主鉴权后执行。页面仅在 \`localhost\` 或 HTTPS 接受管理密钥，浏览器不会持久保存该密钥。

默认配置目录是当前运行用户的 \`UserConfigDir()/cliproxyapi/ofm-providers.json\`，也可通过绝对路径环境变量 \`OFM_PROVIDERS_FILE\` 指定。独立凭据文件可使用 \`OFM_SECRETS_FILE\` 指定绝对路径，禁止放入 Git 版本库。

当前可操作的是 **OpenAI Chat 格式的 API Key/本地免密 Provider**。dsh 的「白嫖模型接入」对应的是额外的账号渠道（Qoder、LobsterAI 等），需要每家适配正式授权、会话刷新、额度接口和模型协议。目前页面的「账号授权渠道」是**标明待开发的展示，不支持账号一键登录或每日签到**。CodeArts/WorkBuddy 不在当前原仓库发布版本的允许清单中，因此没有开放其登录实现。

## 快速开始

```bash
python3 tools/import_dsh_registry.py \
  --input ./data/dsh-registry.snapshot.json \
  --output ./ofm-providers.json \
  --report ./migration-report.json

go test ./...
python3 -m unittest discover -s tests -v
CGO_ENABLED=1 go build -buildmode=c-shared -o ofm-provider.so ./cmd/plugin
```

导入器生成的所有供应商默认禁用。审查服务商 API 条款、base URL 和模型后，单独将对应的 `enabled` 改为 `true`，使用 `key_env` 指定的环境变量提供凭据。

**模型路由重要提示：** CLIProxyAPI 可能要求选到 `ofm-provider` 类型的 auth 记录后才能进入 executor。已提供 `examples/auth-file.ofm-provider.json` 作为待验证的无密钥标记文件，可在隔离测试环境复制到 CLIProxyAPI 的 auth-dir。实际是否正确进入插件须完成 [Issue #1](https://github.com/983033995/cliproxyapi-ofm-provider/issues/1) 验证，不能仅凭该文件推断已可用。API Key 仍由环境变量读取，不写在该文件中。

在 CLIProxyAPI 宿主进程中设置 `OFM_PROVIDERS_FILE=/绝对路径/ofm-providers.json`，并将 `ofm-provider.so` 放入插件目录。参考 `examples/cliproxyapi-config.v8.yaml`。不要将 API Key 提交到版本库。

详细开发边界和联调项见 [ROADMAP.md](ROADMAP.md)。

## 来源及许可证

- dsh commit `9aa84122`（2026-11-11）、MIT 许可；Provider Registry 源自其 `src/providers/registry.json`，并非运行时依赖。
- CLIProxyAPI commit `1aaf56bc`（2026-11-13），`sdk/pluginabi` ABI v1，主分支 JSON Schema v10，实际生产镜像可能不同。
- 当前仅导入元数据，不内嵌原仓库 vendor 二进制、WASM 或账号客户端代码；若分发上游元数据快照，需保留来源和版权信息。

**说明**：静态注册模型不等于可以实际调用；CLIProxyAPI 仍可能在授权选择阶段返回 `auth_not_found`，必须通过独立测试环境验证。
