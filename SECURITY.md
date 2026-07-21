# Security Policy

## Supported versions

Security fixes are applied to the latest commit on `main`.

## Reporting a vulnerability

Please **do not** open a public GitHub issue for security-sensitive reports.

Instead, report privately:

1. Use [GitHub private vulnerability reporting](https://github.com/Obedience-Corp/agent-stream-dbg/security/advisories/new) if available on this repository, or
2. Email the maintainer listed on the [GitHub profile](https://github.com/lancekrogers) associated with this project.

Include:

- A description of the issue and impact
- Steps to reproduce (or a minimal PoC)
- Affected commit or version if known

We will acknowledge receipt when possible and work on a fix before any public disclosure.

## Scope notes

`agent-stream-dbg` is a local CLI/TUI. Typical concerns include:

- Handling of untrusted stream payloads (malformed SSE/gRPC/ACP frames)
- Accidental logging or display of secrets from config or environment
- Path traversal when reading user-supplied config or fixture paths

It does **not** run a network service by default; treat run configs and env files (`.env`) as sensitive on the host.
