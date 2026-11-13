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

## 快速开始

```bash
python3 tools/import_dsh_registry.py \
  --input /path/to/dsh-our-free-model/src/providers/registry.json \
  --output ./ofm-providers.json \
  --report ./migration-report.json

go test ./...
python3 -m unittest discover -s tests -v
CGO_ENABLED=1 go build -buildmode=c-shared -o ofm-provider.so ./cmd/plugin
```

导入器生成的所有供应商默认禁用。审查服务商 API 条款、base URL 和模型后，单独将对应的 `enabled` 改为 `true`，使用 `key_env` 指定的环境变量提供凭据。

在 CLIProxyAPI 宿主进程中设置 `OFM_PROVIDERS_FILE=/绝对路径/ofm-providers.json`，并将 `ofm-provider.so` 放入插件目录。参考 `examples/cliproxyapi-config.v8.yaml`。不要将 API Key 提交到版本库。

详细开发边界和联调项见 [ROADMAP.md](ROADMAP.md)。

## 来源及许可证

- dsh commit `9aa84122`（2026-11-11）、MIT 许可；Provider Registry 源自其 `src/providers/registry.json`，并非运行时依赖。
- CLIProxyAPI commit `1aaf56bc`（2026-11-13），`sdk/pluginabi` ABI v1，主分支 JSON Schema v10，实际生产镜像可能不同。
- 当前仅导入元数据，不内嵌原仓库 vendor 二进制、WASM 或账号客户端代码；若分发上游元数据快照，需保留来源和版权信息。

**说明**：静态注册模型不等于可以实际调用；CLIProxyAPI 仍可能在授权选择阶段返回 `auth_not_found`，必须通过独立测试环境验证。
