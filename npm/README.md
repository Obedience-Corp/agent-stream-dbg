# agent-stream-dbg (npm)

npm distribution of the [agent-stream-dbg](https://github.com/Obedience-Corp/agent-stream-dbg) Go CLI.

On install, a platform binary is downloaded from [GitHub Releases](https://github.com/Obedience-Corp/agent-stream-dbg/releases).

## Install

```bash
npm install -g agent-stream-dbg
# or one-shot
npx agent-stream-dbg --help
```

## Requirements

- Node.js 18+
- macOS or Linux (`x64` / `arm64`)
- Network access to GitHub Releases on install

## Version pinning

```bash
npm install -g agent-stream-dbg@0.1.0
# force a specific release asset tag:
AGENT_STREAM_DBG_VERSION=v0.1.0 npm install -g agent-stream-dbg
```

## Alternatives

```bash
go install github.com/Obedience-Corp/agent-stream-dbg/cmd/agent-stream-dbg@latest
brew install --formula ./homebrew/agent-stream-dbg.rb   # from a clone
```

## Publishing (maintainers)

From the repo root after a tagged release **with platform assets**:

```bash
just release package-binaries v0.1.0   # build + upload assets if not already
cd npm
npm publish --access public
```

Keep `npm/package.json` `version` aligned with the GitHub release (without the leading `v` in package.json).
