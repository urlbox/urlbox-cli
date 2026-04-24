#!/usr/bin/env node

const { execSync } = require("child_process");
const fs = require("fs");
const path = require("path");
const https = require("https");

const REPO = "urlbox/urlbox-cli";
const BINARY = "urlbox";

const PLATFORM_MAP = { darwin: "darwin", linux: "linux", win32: "windows" };
const ARCH_MAP = { x64: "amd64", arm64: "arm64" };

function getPlatform() {
  const p = PLATFORM_MAP[process.platform];
  if (!p) throw new Error(`Unsupported platform: ${process.platform}`);
  return p;
}

function getArch() {
  const a = ARCH_MAP[process.arch];
  if (!a) throw new Error(`Unsupported architecture: ${process.arch}`);
  return a;
}

function getBinaryPath() {
  const ext = process.platform === "win32" ? ".exe" : "";
  return path.join(__dirname, BINARY + ext);
}

function fetch(url) {
  return new Promise((resolve, reject) => {
    https
      .get(url, { headers: { "User-Agent": "urlbox-cli-npm" } }, (res) => {
        if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
          return fetch(res.headers.location).then(resolve, reject);
        }
        if (res.statusCode !== 200) {
          return reject(new Error(`HTTP ${res.statusCode} for ${url}`));
        }
        const chunks = [];
        res.on("data", (c) => chunks.push(c));
        res.on("end", () => resolve(Buffer.concat(chunks)));
        res.on("error", reject);
      })
      .on("error", reject);
  });
}

async function getLatestVersion() {
  const data = await fetch(`https://api.github.com/repos/${REPO}/releases/latest`);
  const release = JSON.parse(data.toString());
  return release.tag_name.replace(/^v/, "");
}

async function main() {
  if (fs.existsSync(getBinaryPath())) return;

  const platform = getPlatform();
  const arch = getArch();
  const version = await getLatestVersion();
  const ext = platform === "windows" ? "zip" : "tar.gz";
  const archive = `${BINARY}_${version}_${platform}_${arch}.${ext}`;
  const url = `https://github.com/${REPO}/releases/download/v${version}/${archive}`;

  console.log(`Downloading ${BINARY} v${version} (${platform}/${arch})...`);
  const data = await fetch(url);

  const tmp = path.join(__dirname, `_tmp_${archive}`);
  fs.writeFileSync(tmp, data);

  if (ext === "zip") {
    execSync(`unzip -o "${tmp}" ${BINARY}.exe -d "${__dirname}"`, { stdio: "ignore" });
  } else {
    execSync(`tar -xzf "${tmp}" -C "${__dirname}" ${BINARY}`, { stdio: "ignore" });
  }

  fs.unlinkSync(tmp);
  fs.chmodSync(getBinaryPath(), 0o755);
  console.log(`${BINARY} v${version} installed.`);
}

main().catch((err) => {
  console.error(`Failed to install ${BINARY}: ${err.message}`);
  process.exit(1);
});
