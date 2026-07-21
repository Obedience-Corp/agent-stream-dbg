#!/usr/bin/env node
"use strict";

/**
 * postinstall: fetch a prebuilt agent-stream-dbg binary from GitHub Releases.
 *
 * Asset naming (from just release package-binaries / CI):
 *   agent-stream-dbg_<version>_<os>_<arch>.tar.gz
 * containing a single executable: agent-stream-dbg
 *
 * Override version with AGENT_STREAM_DBG_VERSION=v0.1.0
 * Override repo with AGENT_STREAM_DBG_REPO=owner/name
 */

const fs = require("fs");
const https = require("https");
const path = require("path");
const { execFileSync } = require("child_process");

const REPO =
  process.env.AGENT_STREAM_DBG_REPO || "Obedience-Corp/agent-stream-dbg";
const pkg = require("../package.json");
const version = normalizeVersion(
  process.env.AGENT_STREAM_DBG_VERSION || pkg.version
);

const platformMap = {
  darwin: "darwin",
  linux: "linux",
};
const archMap = {
  x64: "amd64",
  arm64: "arm64",
};

const platform = platformMap[process.platform];
const arch = archMap[process.arch];
const vendorDir = path.join(__dirname, "..", "vendor");
const binaryName =
  process.platform === "win32" ? "agent-stream-dbg.exe" : "agent-stream-dbg";
const binaryPath = path.join(vendorDir, binaryName);

function normalizeVersion(v) {
  const s = String(v || "").trim();
  if (!s) return "v0.1.0";
  return s.startsWith("v") ? s : `v${s}`;
}

function fail(message) {
  console.error(`[agent-stream-dbg] ${message}`);
  console.error(
    [
      "",
      "Fallbacks:",
      "  go install github.com/Obedience-Corp/agent-stream-dbg/cmd/agent-stream-dbg@latest",
      "  brew install --formula <path-to>/homebrew/agent-stream-dbg.rb",
      "",
    ].join("\n")
  );
  process.exit(1);
}

function download(url, dest) {
  return new Promise((resolve, reject) => {
    const file = fs.createWriteStream(dest);
    const get = (u, redirects = 0) => {
      https
        .get(u, { headers: { "User-Agent": "agent-stream-dbg-npm" } }, (res) => {
          if (
            res.statusCode >= 300 &&
            res.statusCode < 400 &&
            res.headers.location &&
            redirects < 5
          ) {
            res.resume();
            get(res.headers.location, redirects + 1);
            return;
          }
          if (res.statusCode !== 200) {
            res.resume();
            reject(
              new Error(`download failed: HTTP ${res.statusCode} for ${u}`)
            );
            return;
          }
          res.pipe(file);
          file.on("finish", () => file.close(() => resolve()));
        })
        .on("error", reject);
    };
    get(url);
  });
}

async function main() {
  if (!platform || !arch) {
    fail(
      `unsupported platform ${process.platform}/${process.arch}. Use go install or Homebrew instead.`
    );
  }

  // Native binary lives under vendor/; bin/cli.js is the npm entrypoint.
  fs.mkdirSync(vendorDir, { recursive: true });

  const asset = `agent-stream-dbg_${version}_${platform}_${arch}.tar.gz`;
  const url = `https://github.com/${REPO}/releases/download/${version}/${asset}`;
  const tmpDir = fs.mkdtempSync(path.join(require("os").tmpdir(), "asd-npm-"));
  const tarball = path.join(tmpDir, asset);

  console.log(`[agent-stream-dbg] downloading ${asset}…`);
  try {
    await download(url, tarball);
  } catch (err) {
    fail(
      `could not download release asset (${err.message}).\n` +
        `  Ensure release ${version} has platform binaries attached.\n` +
        `  URL: ${url}`
    );
  }

  try {
    execFileSync("tar", ["-xzf", tarball, "-C", tmpDir], { stdio: "pipe" });
  } catch (err) {
    fail(`failed to extract ${asset}: ${err.message}`);
  }

  const candidates = [
    path.join(tmpDir, "agent-stream-dbg"),
    path.join(tmpDir, binaryName),
    path.join(tmpDir, `agent-stream-dbg_${version}_${platform}_${arch}`, "agent-stream-dbg"),
  ];
  const extracted = candidates.find((p) => fs.existsSync(p));
  if (!extracted) {
    fail(`archive did not contain agent-stream-dbg binary (looked in ${tmpDir})`);
  }

  fs.copyFileSync(extracted, binaryPath);
  fs.chmodSync(binaryPath, 0o755);

  // Cleanup temp
  try {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  } catch {
    // ignore
  }

  console.log(`[agent-stream-dbg] installed ${binaryPath}`);
}

main().catch((err) => fail(err.message || String(err)));
