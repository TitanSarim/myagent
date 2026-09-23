#!/usr/bin/env node
"use strict";

/**
 * Thin launcher. Resolves the real myagent binary installed by postinstall.
 */
const { spawn } = require("child_process");
const fs = require("fs");
const path = require("path");

const binName = process.platform === "win32" ? "myagent.exe" : "myagent";
const candidates = [
  path.join(__dirname, binName),
  path.join(__dirname, "..", "vendor", binName),
  path.join(process.env.HOME || process.env.USERPROFILE || "", ".local", "bin", "myagent"),
];

function findBinary() {
  for (const p of candidates) {
    try {
      if (fs.existsSync(p)) return p;
    } catch (_) {}
  }
  return null;
}

const bin = findBinary();
if (!bin) {
  console.error("myagent binary not found. Try reinstalling:");
  console.error("  npm install -g myagent");
  console.error("or:");
  console.error("  curl -fsSL https://raw.githubusercontent.com/TitanSarim/myagent/main/scripts/install.sh | bash");
  process.exit(1);
}

const child = spawn(bin, process.argv.slice(2), {
  stdio: "inherit",
  windowsHide: true,
});
child.on("exit", (code, signal) => {
  if (signal) process.kill(process.pid, signal);
  process.exit(code == null ? 1 : code);
});
