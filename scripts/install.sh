#!/bin/sh
set -e

# Urlbox CLI installer
# Usage: curl -fsSL https://cli.urlbox.com/install.sh | sh

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
  auth_header=""
  if [ -n "${GITHUB_TOKEN:-}" ]; then
    auth_header="-H Authorization: token ${GITHUB_TOKEN}"
  fi
  curl -fsSL $auth_header "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"v?([^"]+)".*/\1/'
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

  url="https://github.com/${REPO}/releases/download/v${version}/urlbox_${version}_${os}_${arch}.${ext}"
  tmpdir="$(mktemp -d)"
  archive="${tmpdir}/urlbox.${ext}"

  info "Downloading ${url}..."
  curl -fsSL "$url" -o "$archive"

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
  rm -rf "$tmpdir"

  info ""
  info "urlbox v${version} installed to ${INSTALL_DIR}/${BINARY_NAME}"
  info ""
  info "Run 'urlbox --help' to get started."
}

main "$@"
