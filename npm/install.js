#!/usr/bin/env node

// Urlbox CLI npm-side installer.
//
// Every release artifact is sha256-pinned in the checksums.txt file
// attached to the GitHub release; checksums.txt is itself sigstore-signed
// (keyless, OIDC-bound to github.com/urlbox/urlbox-cli). This installer
// verifies BOTH:
//   1. sha256 of the downloaded archive against checksums.txt (required)
//   2. sigstore signature on checksums.txt (optional — requires cosign)
//
// Set URLBOX_SKIP_VERIFY=1 to bypass verification (NOT recommended —
// exists only as an escape hatch for offline / air-gapped installs).
//
// Set URLBOX_ALLOW_UNSIGNED=1 to allow a missing sigstore bundle
// (sha256-only verification) on v1.0.0+ releases. By default the bundle
// is required for v1.0.0+ to prevent a network attacker from downgrading
// verification by serving a 404 for the bundle while substituting both
// checksums.txt and the archive. Older 0.x releases predate the bundle
// and don't require it.

const { execFileSync } = require("child_process");
const fs = require("fs");
const path = require("path");
const crypto = require("crypto");
const https = require("https");

const REPO = "urlbox/urlbox-cli";
const BINARY = "urlbox";

const PLATFORM_MAP = { darwin: "darwin", linux: "linux", win32: "windows" };
const ARCH_MAP = { x64: "amd64", arm64: "arm64" };

const COSIGN_IDENTITY_REGEXP = "github.com/urlbox/urlbox-cli";
const COSIGN_OIDC_ISSUER = "https://token.actions.githubusercontent.com";

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

function fetch(url, { allowMissing = false } = {}) {
  return new Promise((resolve, reject) => {
    https
      .get(url, { headers: { "User-Agent": "urlbox-cli-npm" } }, (res) => {
        if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
          return fetch(res.headers.location, { allowMissing }).then(resolve, reject);
        }
        if (res.statusCode === 404 && allowMissing) {
          return resolve(null);
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

function sha256OfBuffer(buf) {
  return crypto.createHash("sha256").update(buf).digest("hex");
}

// Parse a goreleaser checksums.txt body and find the line for `archiveName`.
// Lines are: "<sha256>  <filename>". Returns the expected hex digest, or
// throws if not present.
function expectedSha256For(archiveName, checksumsText) {
  for (const raw of checksumsText.split(/\r?\n/)) {
    const line = raw.trim();
    if (!line) continue;
    const parts = line.split(/\s+/);
    if (parts.length >= 2 && parts[parts.length - 1] === archiveName) {
      return parts[0];
    }
  }
  throw new Error(
    `no checksum entry for ${archiveName} in checksums.txt — the release artifacts may be incomplete`,
  );
}

function verifyCosignBundle(checksumsPath, bundlePath) {
  try {
    execFileSync("cosign", ["version"], { stdio: "ignore" });
  } catch {
    console.log("  cosign not found — skipping signature verification (sha256 already verified).");
    console.log("  Install cosign for cryptographic provenance: https://github.com/sigstore/cosign");
    return;
  }
  try {
    execFileSync(
      "cosign",
      [
        "verify-blob",
        checksumsPath,
        "--bundle",
        bundlePath,
        "--certificate-identity-regexp",
        COSIGN_IDENTITY_REGEXP,
        "--certificate-oidc-issuer",
        COSIGN_OIDC_ISSUER,
      ],
      { stdio: "ignore" },
    );
  } catch (e) {
    throw new Error(
      `sigstore signature verification failed for checksums.txt. The release may have been tampered with — refusing to install. (${e.message})`,
    );
  }
  console.log(`  sigstore signature verified (publisher: ${COSIGN_IDENTITY_REGEXP})`);
}

async function main() {
  if (fs.existsSync(getBinaryPath())) return;

  const platform = getPlatform();
  const arch = getArch();
  const version = await getLatestVersion();
  const ext = platform === "windows" ? "zip" : "tar.gz";
  const archiveName = `${BINARY}_${version}_${platform}_${arch}.${ext}`;
  const base = `https://github.com/${REPO}/releases/download/v${version}`;
  const archiveURL = `${base}/${archiveName}`;

  console.log(`Downloading ${BINARY} v${version} (${platform}/${arch})...`);
  const archiveData = await fetch(archiveURL);
  if (!archiveData) throw new Error(`archive ${archiveName} unavailable`);

  const tmpArchive = path.join(__dirname, `_tmp_${archiveName}`);
  fs.writeFileSync(tmpArchive, archiveData);

  if (process.env.URLBOX_SKIP_VERIFY === "1") {
    // Warnings go to stderr (matches scripts/install.sh and keeps stdout
    // reserved for any future machine-readable install output).
    console.error("================================================================");
    console.error("WARNING: URLBOX_SKIP_VERIFY=1 — bypassing checksum + signature");
    console.error("verification. This is NOT recommended outside of air-gapped");
    console.error("installs where the artifact has been verified out-of-band.");
    console.error("================================================================");
  } else {
    console.log("Verifying release...");
    const checksumsBuf = await fetch(`${base}/checksums.txt`);
    if (!checksumsBuf) {
      fs.unlinkSync(tmpArchive);
      throw new Error("could not download checksums.txt — refusing to install without verification");
    }
    const checksumsText = checksumsBuf.toString("utf8");

    // Sigstore bundle policy:
    //   - v0.x  : bundle is best-effort (older releases predate it).
    //   - v1.0+ : bundle is REQUIRED unless URLBOX_ALLOW_UNSIGNED=1.
    // The "bundle missing → sha256-only" downgrade is exactly what a network
    // attacker would force to defeat publisher-identity verification while
    // serving substituted-but-internally-consistent checksums.txt + archive.
    const allowUnsignedUsed =
      !version.startsWith("0.") && process.env.URLBOX_ALLOW_UNSIGNED === "1";
    const bundleRequired = !version.startsWith("0.") && !allowUnsignedUsed;

    const bundleBuf = await fetch(`${base}/checksums.txt.sigstore.json`, { allowMissing: true });
    if (bundleBuf) {
      const tmpChecksums = path.join(__dirname, "_tmp_checksums.txt");
      const tmpBundle = path.join(__dirname, "_tmp_checksums.txt.sigstore.json");
      fs.writeFileSync(tmpChecksums, checksumsBuf);
      fs.writeFileSync(tmpBundle, bundleBuf);
      try {
        verifyCosignBundle(tmpChecksums, tmpBundle);
      } finally {
        fs.unlinkSync(tmpChecksums);
        fs.unlinkSync(tmpBundle);
      }
    } else if (bundleRequired) {
      fs.unlinkSync(tmpArchive);
      throw new Error(
        `sigstore bundle (checksums.txt.sigstore.json) missing for v${version}. Refusing to downgrade to sha256-only — set URLBOX_ALLOW_UNSIGNED=1 to override (NOT recommended for v1.0+ releases).`,
      );
    } else if (allowUnsignedUsed) {
      console.error("================================================================");
      console.error(`WARNING: URLBOX_ALLOW_UNSIGNED=1 — downgrading to sha256-only`);
      console.error(`on v${version}. A network attacker who substitutes both`);
      console.error("checksums.txt AND the archive bypasses publisher-identity");
      console.error("verification. Use only when you cannot fetch the sigstore");
      console.error("bundle (corporate proxy, etc.).");
      console.error("================================================================");
    } else {
      console.log("  no sigstore bundle on this release — sha256-only verification");
    }

    const expected = expectedSha256For(archiveName, checksumsText);
    const actual = sha256OfBuffer(archiveData);
    if (expected !== actual) {
      fs.unlinkSync(tmpArchive);
      throw new Error(
        `checksum mismatch for ${archiveName}: expected ${expected}, got ${actual}. Artifact may have been tampered with.`,
      );
    }
    console.log(`  sha256 verified: ${actual}`);
  }

  if (ext === "zip") {
    execFileSync("unzip", ["-o", tmpArchive, `${BINARY}.exe`, "-d", __dirname], { stdio: "ignore" });
  } else {
    execFileSync("tar", ["-xzf", tmpArchive, "-C", __dirname, BINARY], { stdio: "ignore" });
  }

  fs.unlinkSync(tmpArchive);
  fs.chmodSync(getBinaryPath(), 0o755);
  console.log(`${BINARY} v${version} installed.`);
}

main().catch((err) => {
  console.error(`Failed to install ${BINARY}: ${err.message}`);
  process.exit(1);
});
