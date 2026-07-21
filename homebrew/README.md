# Homebrew distribution

**Production installs** use the org tap, updated automatically by GoReleaser on
each tag (same pattern as Festival):

```bash
brew tap Obedience-Corp/tap
brew trust Obedience-Corp/tap   # Homebrew 6+ once
brew install agent-stream-dbg
```

Formula path in the tap:  
https://github.com/Obedience-Corp/homebrew-tap/blob/main/Formula/agent-stream-dbg.rb

## Formula vs Cask

| | **Formula** | **Cask** |
|--|-------------|----------|
| For | CLIs / TUIs | macOS GUI `.app` |
| This tool | **Yes** | No |

A **tap** is just a third-party formula/cask repo. Festival puts GUI helpers under
`Casks/`; agent-stream-dbg is a terminal tool under `Formula/`.

## In-repo source formula

[`agent-stream-dbg.rb`](./agent-stream-dbg.rb) builds from a GitHub source
tarball (useful for local brew testing). Production users should use the tap.

```bash
brew install --formula ./homebrew/agent-stream-dbg.rb
```

## Release automation

On `git push origin vX.Y.Z`, CI runs GoReleaser which:

1. Builds multi-platform archives + `checksums.txt`
2. Creates/updates the GitHub Release
3. Commits an updated Formula to `Obedience-Corp/homebrew-tap`
4. Publishes `@obedience-corp/agent-stream-dbg` to npm

Requires repo secret `HOMEBREW_TAP_GITHUB_TOKEN` (write to the tap).
