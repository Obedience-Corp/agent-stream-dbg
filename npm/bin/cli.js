#!/usr/bin/env node
"use strict";

const { spawn } = require("child_process");
const fs = require("fs");
const path = require("path");

const binaryName =
  process.platform === "win32" ? "agent-stream-dbg.exe" : "agent-stream-dbg";
const binaryPath = path.join(__dirname, "..", "vendor", binaryName);

if (!fs.existsSync(binaryPath)) {
  console.error(
    [
      "agent-stream-dbg binary is missing.",
      "The postinstall step may have failed.",
      "",
      "Try reinstalling:",
      "  npm install -g agent-stream-dbg",
      "",
      "Or install with Go:",
      "  go install github.com/Obedience-Corp/agent-stream-dbg/cmd/agent-stream-dbg@latest",
    ].join("\n")
  );
  process.exit(1);
}

const child = spawn(binaryPath, process.argv.slice(2), {
  stdio: "inherit",
  windowsHide: true,
});

child.on("error", (err) => {
  console.error(`failed to start agent-stream-dbg: ${err.message}`);
  process.exit(1);
});

child.on("exit", (code, signal) => {
  if (signal) {
    process.kill(process.pid, signal);
    return;
  }
  process.exit(code === null ? 1 : code);
});
