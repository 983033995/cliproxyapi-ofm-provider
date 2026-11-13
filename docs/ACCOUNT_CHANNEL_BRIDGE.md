# Account Channel Bridge: CLIProxyAPI ↔ Our Free Model Standalone

## Core integration

The priority is **account-authenticated model providers** (LobsterAI, Qoder/Qoder CN, TRAE, Cline, Loomy, Raccoon, MiniMax Code, ZCode, Gemini Code Assist), NOT API Key aggregator setup.

DSH maintains a stand-alone service with a local web console, account login, refresh, quota and some check-in flows, and the endpoints GET /v1/models, POST /v1/chat/completions and POST /v1/responses. Rather than rewrite ten non-standard protocols, this plugin acts as a client of that local standalone runtime.

Upstream documentation: https://github.com/Ebony-Vinyl/dsh-our-free-model/blob/main/packages/standalone/README.md

### Architecture

1. User starts standalone service on localhost and signs into accounts through its own web console.
2. User enters its **Forward Key** (standalone settings.json -> forwardKey) in the CLIProxyAPI plugin's "账号授权渠道" management page.
3. Plugin calls authenticated GET /v1/models using host.http.do and filters to the named account channel model namespaces. It does not copy anonymous, Kilo, EAC or arbitrary API provider models.
4. Plugin caches those model ids in a local 0600 config. A sidecar model id such as qoder/qfmodel is exposed to CLIProxyAPI as ofm-account/qoder/qfmodel. Embedded slashes in upstream model ids are preserved.
5. Plugin model.register publishes these names, and executor.execute / execute_stream forwards chat requests to the local standalone service through host HTTP callbacks.
6. The source runtime owns account selection, OAuth/password/refresh token, quota and provider-specific wire translation. The plugin only owns the standalone Forward Key and routing.

**This is a sidecar-backed MVP; not a native login implementation or a single-binary release.** The source channel pack is MIT attributed to Jet (vendor/channel-pack/LICENSE, NOTICE.md). This plugin does not vendor it.

## How to develop and test

Same machine is REQUIRED for both services. A Docker container has a different localhost and is not yet supported.

In a trusted source checkout, follow the original standalone README and Node.js engine requirements:

~~~sh
git clone https://github.com/Ebony-Vinyl/dsh-our-free-model.git
cd dsh-our-free-model
npm ci
npm run start:standalone -- --port 18900
~~~

Use the actual port printed by the service (it may choose another port). Open the printed one-time login link, add your own account(s) in 免费账号渠道, and verify their models in the original app. Get the standalone **Forward Key** from its API 接入 page or the local settings.json file. Do not copy any account cookies, refresh tokens, or local credentials to CLIProxyAPI.

In this plugin repository:

~~~sh
go test ./...
CGO_ENABLED=1 go build -buildmode=c-shared -o ofm-provider.so ./cmd/plugin
~~~

Deploy the .so in the CLIProxyAPI v8 plugins directory and enable the host plugin. On localhost, open the plugin's browser resource:

http://127.0.0.1:8317/v0/resource/plugins/ofm-provider/dashboard

Authenticate with the CLIProxyAPI **Management Key**, choose 账号授权渠道, enter the local standalone address and Forward Key, click 连接并同步模型. If you sign in more accounts or their model catalogs change, choose 重新同步 and reload the CLIProxyAPI plugin/host so static model registration can refresh.

Use **CLIProxyAPI's own caller API Key** to call /v1/chat/completions with a model id beginning ofm-account/. Do not use the standalone Forward Key for this client. The standalone key is for the plugin's private upstream hop.

## Security and limitations

- Account gateway only accepts http://127.0.0.1:PORT, localhost:PORT or [::1]:PORT; it rejects remote IPs, credentials in URL, query/fragment, and non-root paths. It requires a valid configured Forward Key.
- Forward Key is stored as plaintext in a chmod 0600 local file (NOT encrypted); it is never returned by the dashboard APIs. Secure the OS user account and backup storage.
- Account login and check-in happen **only** in the standalone app; account card states are derived from synced model metadata. Do not interpret synced models as proof of a current login or usable account balance.
- Disconnecting this bridge removes only the plugin's saved Forward Key and models; it does NOT sign out upstream accounts in the standalone service.
- The original ten account channels include reverse-engineered private client protocol implementations. Auth/API behavior can change and account terms may restrict third-party automated access. No protection-bypass or client impersonation has been reimplemented in this plugin.
- The upstream standalone distribution is not formally released. Pin versions, review dependencies and source licenses, and evaluate security before use.
- Running tests / compiling does NOT prove real CLIProxyAPI v8 end-to-end authentication, /v1/models registration or SSE forwarding. Finish Issue #1 with an isolated host and a real authorized account before claiming production support.

## Planned future improvement

A managed SidecarManager could install and supervise an independently licensed, pinned standalone runtime, expose status, and deep-link to its account login page. A true plugin-owned native auth.login.start/auth.login.poll/auth.refresh adapter per provider should be attempted only where authorized and supported by stable official protocols.
