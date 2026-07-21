# Homebrew distribution

## Install (recommended — org tap)

Prebuilt binaries from GitHub Releases, published in
[Obedience-Corp/homebrew-tap](https://github.com/Obedience-Corp/homebrew-tap):

```bash
brew tap Obedience-Corp/tap
brew trust Obedience-Corp/tap   # Homebrew 6+ (once per machine)
brew install agent-stream-dbg
```

## Install (this repo — source build)

Useful for development. Formula: [`agent-stream-dbg.rb`](./agent-stream-dbg.rb)

```bash
git clone https://github.com/Obedience-Corp/agent-stream-dbg.git
cd agent-stream-dbg
brew install --formula ./homebrew/agent-stream-dbg.rb
```

## Formula vs Cask

| | **Formula** | **Cask** |
|--|-------------|----------|
| For | CLIs, libraries, daemons, TUIs | macOS GUI `.app` bundles |
| Install | `brew install name` | `brew install --cask name` |
| This tool | **Yes** | No |

Homebrew “taps” are third-party formula/cask repos. A **tap** can contain both
`Formula/` and `Casks/`. **agent-stream-dbg** is a terminal debugger, so it
ships as a **Formula**, not a cask.

## After a release

1. Upload platform assets: `just release package-binaries vX.Y.Z`
2. Update `Obedience-Corp/homebrew-tap` → `Formula/agent-stream-dbg.rb`
   (version, URLs, sha256s)
3. Optionally refresh the in-repo source formula sha256:
   `just release homebrew-sha vX.Y.Z`
