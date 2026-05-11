#!/bin/sh
set -eu

# Urlbox CLI installer
# Usage: curl -fsSL https://cli.urlbox.com/install.sh | sh
#
# Every release artifact is sha256-pinned in the checksums.txt file attached
# to the GitHub release; checksums.txt is itself sigstore-signed (keyless,
# OIDC-bound to github.com/urlbox/urlbox-cli). This installer verifies BOTH:
#   1. sha256 of the downloaded archive against checksums.txt (required)
#   2. sigstore signature on checksums.txt (optional — requires cosign)
#
# Set URLBOX_SKIP_VERIFY=1 to bypass verification (NOT recommended — exists
# only as an escape hatch for offline / air-gapped installs where the user
# has out-of-band-verified the artifact themselves).
#
# Set URLBOX_ALLOW_UNSIGNED=1 to allow a missing sigstore bundle (sha256-only
# verification) on v1.0.0+ releases. By default the bundle is required for
# v1.0.0+ to prevent a network attacker from downgrading verification by
# serving a 404 for the bundle while substituting both checksums.txt and
# the archive. Older 0.x releases predate the bundle and don't require it.

REPO="urlbox/urlbox-cli"
INSTALL_DIR="/usr/local/bin"
BINARY_NAME="urlbox"

# Detect non-interactive environment
is_interactive() {
  [ -t 0 ] && [ -t 1 ] && [ -z "${CI:-}" ] && [ -z "${GITHUB_ACTIONS:-}" ] && [ -z "${URLBOX_SKIP_SETUP:-}" ]
}

info() {
  printf '%s\n' "$1" >&2
}

error() {
  printf 'Error: %s\n' "$1" >&2
  exit 1
}

detect_os() {
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  case "$os" in
    darwin) echo "darwin" ;;
    linux) echo "linux" ;;
    mingw*|msys*|cygwin*) echo "windows" ;;
    *) error "unsupported OS: $os" ;;
  esac
}

detect_arch() {
  arch="$(uname -m)"
  case "$arch" in
    x86_64|amd64) echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    *) error "unsupported architecture: $arch" ;;
  esac
}

latest_version() {
  # Use redirect to avoid GitHub API rate limits (shared CI IPs get 403)
  redirect=$(curl -sS -o /dev/null -w '%{redirect_url}' "https://github.com/${REPO}/releases/latest" 2>/dev/null)
  if [ -n "$redirect" ]; then
    echo "$redirect" | sed 's|.*/v||'
    return
  fi
  # Fallback to API with auth if available
  if [ -n "${GITHUB_TOKEN:-}" ]; then
    curl -fsSL -H "Authorization: token ${GITHUB_TOKEN}" "https://api.github.com/repos/${REPO}/releases/latest" \
      | grep '"tag_name"' | sed -E 's/.*"v?([^"]+)".*/\1/'
  else
    curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
      | grep '"tag_name"' | sed -E 's/.*"v?([^"]+)".*/\1/'
  fi
}

# Compute the sha256 of $1 using sha256sum (GNU/Linux) or shasum (BSD/macOS).
compute_sha256() {
  file="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$file" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$file" | awk '{print $1}'
  else
    error "neither sha256sum nor shasum is available; cannot verify checksum. Install coreutils (sha256sum) or perl (shasum), or set URLBOX_SKIP_VERIFY=1 to bypass (not recommended)."
  fi
}

# Verify $archive against the entry for its basename in $checksums.
verify_archive_sha256() {
  archive="$1"
  checksums="$2"
  name=$(basename "$archive")
  # checksums.txt lines are: "<sha256>  <filename>"
  expected=$(awk -v n="$name" '$2 == n {print $1}' "$checksums" | head -1)
  if [ -z "$expected" ]; then
    error "no checksum entry for ${name} in checksums.txt — the release artifacts may be incomplete"
  fi
  actual=$(compute_sha256 "$archive")
  if [ "$expected" != "$actual" ]; then
    error "checksum mismatch for ${name}: expected ${expected}, got ${actual}. Artifact may have been tampered with."
  fi
  info "  sha256 verified: ${actual}"
}

# Verify the sigstore bundle on checksums.txt against the urlbox-cli OIDC
# identity. Soft-optional: if cosign is not installed, prints a hint and
# returns successfully (the sha256 check already gave integrity; cosign
# adds publisher-identity provenance).
verify_cosign_bundle() {
  checksums="$1"
  bundle="$2"
  if ! command -v cosign >/dev/null 2>&1; then
    info "  cosign not found — skipping signature verification (sha256 already verified)."
    info "  Install cosign for cryptographic provenance: https://github.com/sigstore/cosign"
    return
  fi
  if ! cosign verify-blob "$checksums" \
      --bundle "$bundle" \
      --certificate-identity-regexp='github.com/urlbox/urlbox-cli' \
      --certificate-oidc-issuer='https://token.actions.githubusercontent.com' \
      >/dev/null 2>&1; then
    error "sigstore signature verification failed for checksums.txt. The release may have been tampered with — refusing to install."
  fi
  info "  sigstore signature verified (publisher: github.com/urlbox/urlbox-cli)"
}

main() {
  os="$(detect_os)"
  arch="$(detect_arch)"
  version="$(latest_version)"

  if [ -z "$version" ]; then
    error "could not determine latest version"
  fi

  info "Installing urlbox v${version} (${os}/${arch})..."

  ext="tar.gz"
  if [ "$os" = "windows" ]; then
    ext="zip"
  fi

  archive_name="urlbox_${version}_${os}_${arch}.${ext}"
  base="https://github.com/${REPO}/releases/download/v${version}"

  tmpdir="$(mktemp -d)"
  trap 'rm -rf "$tmpdir"' EXIT
  archive="${tmpdir}/${archive_name}"
  checksums="${tmpdir}/checksums.txt"
  bundle="${tmpdir}/checksums.txt.sigstore.json"

  info "Downloading ${archive_name}..."
  curl -fsSL "${base}/${archive_name}" -o "$archive"

  if [ "${URLBOX_SKIP_VERIFY:-}" = "1" ]; then
    info "================================================================"
    info "WARNING: URLBOX_SKIP_VERIFY=1 — bypassing checksum + signature"
    info "verification. This is NOT recommended outside of air-gapped"
    info "installs where the artifact has been verified out-of-band."
    info "================================================================"
  else
    info "Verifying release..."
    curl -fsSL "${base}/checksums.txt" -o "$checksums" \
      || error "could not download checksums.txt — refusing to install without verification"

    # Sigstore bundle policy:
    #   - v0.x  : bundle is best-effort (older releases predate it).
    #   - v1.0+ : bundle is REQUIRED unless URLBOX_ALLOW_UNSIGNED=1.
    # The "bundle missing → sha256-only" downgrade is exactly what a network
    # attacker would force to defeat publisher-identity verification while
    # serving substituted-but-internally-consistent checksums.txt + archive.
    case "$version" in
      0.*) bundle_required=0 ;;
      *)   bundle_required=1 ;;
    esac
    if [ "${URLBOX_ALLOW_UNSIGNED:-}" = "1" ]; then
      bundle_required=0
    fi

    if curl -fsSL "${base}/checksums.txt.sigstore.json" -o "$bundle" 2>/dev/null; then
      verify_cosign_bundle "$checksums" "$bundle"
    elif [ "$bundle_required" = "1" ]; then
      error "sigstore bundle (checksums.txt.sigstore.json) missing for v${version}. Refusing to downgrade to sha256-only — set URLBOX_ALLOW_UNSIGNED=1 to override (NOT recommended for v1.0+ releases)."
    else
      info "  no sigstore bundle on this release — sha256-only verification"
    fi

    verify_archive_sha256 "$archive" "$checksums"
  fi

  info "Extracting..."
  if [ "$ext" = "zip" ]; then
    unzip -q "$archive" -d "$tmpdir"
  else
    tar -xzf "$archive" -C "$tmpdir"
  fi

  if [ -w "$INSTALL_DIR" ]; then
    mv "${tmpdir}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
  else
    info "Need sudo to install to ${INSTALL_DIR}"
    sudo mv "${tmpdir}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
  fi

  chmod +x "${INSTALL_DIR}/${BINARY_NAME}"

  info ""
  info "urlbox v${version} installed to ${INSTALL_DIR}/${BINARY_NAME}"
  info ""
  info "Run 'urlbox --help' to get started."
}

main "$@"
