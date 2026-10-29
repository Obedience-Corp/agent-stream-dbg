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
just build       # build-util vet + build dashboard
just test        # build-util short-mode test dashboard
just test unit   # full suite; same coverage as go test ./...
```

`just --list` shows available recipes (demos, VHS recordings, release helpers).
Build/test/vet dashboards use [build-util](https://github.com/Obedience-Corp/build-util)
through `internal/buildutil/main.go`. `just test vet` invokes its Go vet dashboard;
`just test lint` retains the repository's additional lint checks. The default
test dashboard runs with `-short`; the recipe also runs the dialect tests that
build-util v0.1.0 does not discover (they use an external test package).
`just test unit` and `just test all` run the full suite. CI and release validation keep using the full suite. For detailed
subprocess output, run `go run ./internal/buildutil -v build` (or `test`/`lint`).

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

## Passive observer modules

Keep source decoding in the mapping/dialect layer. In `internal/visualizer`,
`observer.go` owns model updates; `observer_view.go` owns rendering and clipping.
`observer_history.go` owns selection and the single-worker request lifecycle;
`observer_history_file.go` owns bounded framing, source validation and disk
lookup. History tests separate navigation, file recovery and memory budgets.
Keep new maintained code files below 500 lines by separating responsibilities,
without splitting generated fixture/golden output or refactoring unrelated code.
