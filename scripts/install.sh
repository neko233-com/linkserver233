#!/usr/bin/env bash
set -euo pipefail

VERSION="${1:-latest}"
REPO="neko233-com/linkserver233"
BINARY_NAME="linkserver233"

detect_os() {
  case "$(uname -s)" in
    Linux) echo "linux" ;;
    Darwin) echo "darwin" ;;
    *) echo "unsupported" ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64) echo "amd64" ;;
    arm64|aarch64) echo "arm64" ;;
    *) echo "unsupported" ;;
  esac
}

normalize_version() {
  local value="$1"
  value="${value#v}"
  value="${value#V}"
  printf '%s' "$value"
}

main() {
  local os arch asset url install_dir tmp_dir target version_label

  os="$(detect_os)"
  arch="$(detect_arch)"
  if [ "$os" = "unsupported" ]; then
    echo "Unsupported operating system. Use install.ps1 on Windows." >&2
    exit 1
  fi
  if [ "$arch" = "unsupported" ]; then
    echo "Unsupported architecture: $(uname -m)" >&2
    exit 1
  fi

  if [ "$VERSION" = "latest" ] || [ -z "$VERSION" ]; then
    version_label="latest"
  else
    VERSION="$(normalize_version "$VERSION")"
    version_label="v$VERSION"
  fi

  asset="${BINARY_NAME}-${os}-${arch}"
  if [ "$version_label" = "latest" ]; then
    url="https://github.com/${REPO}/releases/latest/download/${asset}"
  else
    url="https://github.com/${REPO}/releases/download/${version_label}/${asset}"
  fi

  install_dir="/usr/local/bin"
  target="${install_dir}/${BINARY_NAME}"
  tmp_dir="$(mktemp -d)"

  echo "Installing ${BINARY_NAME} ${version_label} for ${os}/${arch}..."
  echo "Downloading ${url}..."
  curl -fsSL "$url" -o "${tmp_dir}/${BINARY_NAME}"
  chmod +x "${tmp_dir}/${BINARY_NAME}"

  if [ -w "$install_dir" ]; then
    mv -f "${tmp_dir}/${BINARY_NAME}" "$target"
  else
    sudo mv -f "${tmp_dir}/${BINARY_NAME}" "$target"
  fi

  rm -rf "$tmp_dir"

  echo "Installed to ${target}"
  echo "Run: ${BINARY_NAME} version"
}

main "$@"
