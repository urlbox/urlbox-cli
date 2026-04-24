#!/usr/bin/env node

const { spawnSync } = require("child_process");
const path = require("path");

const ext = process.platform === "win32" ? ".exe" : "";
const bin = path.join(__dirname, "urlbox" + ext);
const result = spawnSync(bin, process.argv.slice(2), { stdio: "inherit" });

if (result.error) {
  if (result.error.code === "ENOENT") {
    console.error("urlbox binary not found. Try reinstalling: npm install -g @urlbox/cli");
  } else {
    console.error(result.error.message);
  }
  process.exit(1);
}

process.exit(result.status);
