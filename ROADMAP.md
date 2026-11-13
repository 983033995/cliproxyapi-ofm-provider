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
