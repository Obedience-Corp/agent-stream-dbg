# Contributing

Thanks for your interest in `agent-stream-dbg`.

## Development setup

Requirements:

- Go (see `go.mod`)
- Optional: [just](https://github.com/casey/just) for project recipes

```bash
git clone https://github.com/Obedience-Corp/agent-stream-dbg.git
cd agent-stream-dbg
just deps    # or: go mod download
just build
just test    # or: go test ./...
```

`just --list` shows available recipes (demos, VHS recordings, release helpers).

## Project shape

| Piece | Role |
|-------|------|
| **Transport** | How bytes arrive (`sse`, `grpc`, `acp`, `replay`) |
| **Dialect** | YAML mapping: what the bytes mean (agents, tools, content) |
| **TUI / CLI** | How you watch, explain, timeline, and replay |

Adding support for a new multi-agent system should prefer a **dialect YAML** (and fixtures) over new Go packages. Use `agent-stream-dbg init` / `explain` when drafting dialects.

Private-backend names (e.g. Brainyard personas) may appear in dialect **data** and fixtures, but must not be hardcoded into executable Go under `internal/` or `cmd/`. `just test lint-seam` enforces that.

## Pull requests

1. Keep changes focused and tested (`go test ./...`).
2. Prefer behavior-oriented tests over brittle UI snapshots.
3. Update README or examples when user-facing behavior changes.
4. Do not commit secrets, real API keys, or private host paths.

Issues and PRs are welcome. There is no guarantee of review turnaround; this is maintained as a small open-source tool.

## Distribution packages

Same model as Festival:

| Channel | How it ships |
|---------|----------------|
| GitHub Releases | GoReleaser archives + `checksums.txt` |
| Homebrew | GoReleaser updates `Obedience-Corp/homebrew-tap` **Formula** (not Cask) |
| npm | `@obedience-corp/agent-stream-dbg` published after GoReleaser |

Release: `just release tag vX.Y.Z` (or `create`). Requires secrets
`HOMEBREW_TAP_GITHUB_TOKEN` and `NPM_TOKEN` on the GitHub repo.

## Security

See [SECURITY.md](SECURITY.md) for private vulnerability reporting.
