# Developer instructions

This is an independent CLIProxyAPI v8 native plugin, NOT a fork of CLIProxyAPI.

- Keep Provider data, auth references, and HTTP transport separate.
- Preserve upstream model IDs containing slashes.
- Do not assume static models can execute without a host auth record.
- Never commit credentials, user session cookies, OAuth refresh tokens or compiled dynamic libraries.
- Prefer authorized API Key providers and localhost no-Key services.
- Do not implement impersonation of protected upstream client identities, restriction circumvention or unofficial account-login automation.
- Preserve MIT source notices for any data or code imported from dsh.
- Before release, run real host integration, SSE error/close tests and platform build checks.
