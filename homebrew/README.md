# Homebrew distribution

Formula: [`agent-stream-dbg.rb`](./agent-stream-dbg.rb)

## Install (from this repo)

```bash
git clone https://github.com/Obedience-Corp/agent-stream-dbg.git
cd agent-stream-dbg
brew install --formula ./homebrew/agent-stream-dbg.rb
```

## Install (org tap — recommended for users)

1. Create (or reuse) `Obedience-Corp/homebrew-tap`.
2. Copy `homebrew/agent-stream-dbg.rb` into that repo as `Formula/agent-stream-dbg.rb`.
3. Users run:

```bash
brew tap Obedience-Corp/tap
brew install agent-stream-dbg
```

## After a release

Update `url` and `sha256` in the formula:

```bash
just release homebrew-sha v0.1.1
# paste the printed sha256 into homebrew/agent-stream-dbg.rb
```

Or:

```bash
curl -sL https://github.com/Obedience-Corp/agent-stream-dbg/archive/refs/tags/vX.Y.Z.tar.gz | shasum -a 256
```
