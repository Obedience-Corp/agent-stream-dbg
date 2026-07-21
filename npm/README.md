# @obedience-corp/agent-stream-dbg

npm distribution of the [agent-stream-dbg](https://github.com/Obedience-Corp/agent-stream-dbg) Go CLI.

Same model as [`@obedience-corp/festival`](https://www.npmjs.com/package/@obedience-corp/festival):
on install, download the matching GitHub Release archive and verify `checksums.txt`.

## Install

```bash
npm install -g @obedience-corp/agent-stream-dbg
npx @obedience-corp/agent-stream-dbg --help
```

Also works with pnpm / bun.

## Requirements

- Node.js 18+
- macOS or Linux (`x64` / `arm64`)
- Network access to GitHub Releases on install

## Alternatives

```bash
go install github.com/Obedience-Corp/agent-stream-dbg/cmd/agent-stream-dbg@latest

brew tap Obedience-Corp/tap
brew trust Obedience-Corp/tap
brew install agent-stream-dbg
```

## Maintainers

Releases are automated via GoReleaser + this package’s `scripts/publish_npm_package.sh`
(on tag push). Do not hand-edit package version on `main` (stays `0.0.0`); publish
sets the version from the git tag.
