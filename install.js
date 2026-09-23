#!/usr/bin/env node
"use strict";

/**
 * npm postinstall: download a release binary, or build from source with Go.
 */
const fs = require("fs");
const path = require("path");
const https = require("https");
const { execSync, spawnSync } = require("child_process");

const pkg = require("./package.json");
const VERSION = pkg.version;
const REPO = "TitanSarim/myagent";

const platformMap = { darwin: "darwin", linux: "linux", win32: "windows" };
const archMap = { x64: "amd64", arm64: "arm64" };

const platform = platformMap[process.platform];
const arch = archMap[process.arch];
const binDir = path.join(__dirname, "npm", "bin");
const binName = process.platform === "win32" ? "myagent.exe" : "myagent";
const dest = path.join(binDir, binName);

function log(msg) {
  console.log(`[myagent] ${msg}`);
}

function download(url, outPath) {
  return new Promise((resolve, reject) => {
    const file = fs.createWriteStream(outPath);
    const follow = (u, redirects) => {
      https
        .get(u, (res) => {
          if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
            if (redirects > 5) return reject(new Error("too many redirects"));
            res.resume();
            return follow(res.headers.location, redirects + 1);
          }
          if (res.statusCode !== 200) {
            res.resume();
            return reject(new Error(`HTTP ${res.statusCode} for ${u}`));
          }
          res.pipe(file);
          file.on("finish", () => file.close(() => resolve()));
        })
        .on("error", reject);
    };
    follow(url, 0);
  });
}

async function tryDownloadRelease() {
  if (!platform || !arch) return false;
  const ext = process.platform === "win32" ? ".exe" : "";
  const urls = [
    `https://github.com/${REPO}/releases/download/v${VERSION}/myagent_${VERSION}_${platform}_${arch}${ext}`,
    `https://github.com/${REPO}/releases/download/v${VERSION}/localcode_${VERSION}_${platform}_${arch}${ext}`,
  ];
  fs.mkdirSync(binDir, { recursive: true });
  for (const url of urls) {
    try {
      log(`downloading ${url}`);
      await download(url, dest);
      if (process.platform !== "win32") fs.chmodSync(dest, 0o755);
      log(`installed binary → ${dest}`);
      return true;
    } catch (e) {
      log(`release miss: ${e.message}`);
    }
  }
  return false;
}

function tryGoBuild() {
  const go = spawnSync("go", ["version"], { encoding: "utf8" });
  if (go.status !== 0) return false;

  log("building with Go (go install)…");
  try {
    const gopathBin =
      execSync("go env GOPATH", { encoding: "utf8" }).trim() +
      (process.platform === "win32" ? "\\bin" : "/bin");
    try {
      execSync(`go install github.com/${REPO}/cmd/localcode@v${VERSION}`, {
        stdio: "inherit",
        env: process.env,
      });
    } catch (_) {
      execSync(`go install github.com/${REPO}/cmd/localcode@latest`, {
        stdio: "inherit",
        env: process.env,
      });
    }
    const src = path.join(gopathBin, process.platform === "win32" ? "localcode.exe" : "localcode");
    if (!fs.existsSync(src)) return false;
    fs.mkdirSync(binDir, { recursive: true });
    fs.copyFileSync(src, dest);
    if (process.platform !== "win32") fs.chmodSync(dest, 0o755);
    log(`built → ${dest}`);
    return true;
  } catch (e) {
    log(`go install failed: ${e.message}`);
    return false;
  }
}

(async () => {
  try {
    if (fs.existsSync(dest)) {
      log("binary already present");
      return;
    }
    const ok = (await tryDownloadRelease()) || tryGoBuild();
    if (!ok) {
      console.warn(`[myagent] could not install a binary automatically.`);
      console.warn(
        `[myagent] Install with: curl -fsSL https://raw.githubusercontent.com/${REPO}/main/scripts/install.sh | bash`
      );
    }
  } catch (e) {
    console.warn(`[myagent] postinstall warning: ${e.message}`);
  }
})();
