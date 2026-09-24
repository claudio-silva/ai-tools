#!/usr/bin/env node
// aitools npm launcher — runs the bundled Go binary for this platform.
"use strict";

const { spawnSync } = require("node:child_process");
const path = require("node:path");
const fs = require("node:fs");

const platforms = { darwin: "darwin" };
const arches = { arm64: "arm64", x64: "amd64" };

const platform = platforms[process.platform];
const arch = arches[process.arch];
if (!platform || !arch) {
  console.error(
    `aitools: unsupported platform ${process.platform}/${process.arch}. ` +
      "The npm package currently ships macOS binaries only — see " +
      "https://github.com/claudio-silva/ai-tools for other ways to install."
  );
  process.exit(1);
}

const binary = path.join(__dirname, `aitools-${platform}-${arch}`);
if (!fs.existsSync(binary)) {
  console.error(`aitools: missing bundled binary ${path.basename(binary)}`);
  process.exit(1);
}

const result = spawnSync(binary, process.argv.slice(2), { stdio: "inherit" });
if (result.error) {
  console.error(`aitools: ${result.error.message}`);
  process.exit(1);
}
if (result.signal) {
  process.kill(process.pid, result.signal);
}
process.exit(result.status === null ? 1 : result.status);
