# Homebrew formula for agent-stream-dbg (source build).
#
# Install from a local clone:
#   brew install --formula ./homebrew/agent-stream-dbg.rb
#
# Or via an org tap that vendors this formula:
#   brew tap Obedience-Corp/tap
#   brew install agent-stream-dbg
#
# After each release, update url/sha256 (or run: just release homebrew-sha VERSION).

class AgentStreamDbg < Formula
  desc "TUI debugger for multi-agent event streams (SSE, gRPC, ACP)"
  homepage "https://github.com/Obedience-Corp/agent-stream-dbg"
  url "https://github.com/Obedience-Corp/agent-stream-dbg/archive/refs/tags/v0.1.0.tar.gz"
  sha256 "bf8ed58e4933cf885c42e4da3bc4d8932a54aa97cbd4bf5b8a6e83944be6bd8b"
  license "Apache-2.0"
  head "https://github.com/Obedience-Corp/agent-stream-dbg.git", branch: "main"

  depends_on "go" => :build

  def install
    ENV["CGO_ENABLED"] = "0"
    system "go", "build",
           *std_go_args(ldflags: "-s -w"),
           "./cmd/agent-stream-dbg"
  end

  test do
    assert_match "agent-stream-dbg", shell_output("#{bin}/agent-stream-dbg --help")
  end
end
