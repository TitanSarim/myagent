#!/usr/bin/env bash
# One-line install for myagent (Linux / macOS)
#   curl -fsSL https://raw.githubusercontent.com/TitanSarim/myagent/main/scripts/install.sh | bash
#   curl -fsSL … | bash -s -- myagent
set -euo pipefail

NAME="${1:-myagent}"
REPO="TitanSarim/myagent"
VERSION="${MYAGENT_VERSION:-0.4.0}"
BIN_DIR="${HOME}/.local/bin"
DEST="${BIN_DIR}/${NAME}"

if [[ ! "$NAME" =~ ^[a-zA-Z][a-zA-Z0-9_-]*$ ]]; then
  echo "error: invalid command name: $NAME" >&2
  exit 1
fi

mkdir -p "$BIN_DIR"
export PATH="${HOME}/.local/go/bin:${BIN_DIR}:${PATH}"

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) echo "unsupported arch: $arch" >&2; exit 1 ;;
esac
case "$os" in
  linux|darwin) ;;
  *) echo "unsupported OS: $os (use Windows binary from Releases)" >&2; exit 1 ;;
esac

download_release() {
  local url asset
  for asset in \
    "myagent_${VERSION}_${os}_${arch}" \
    "localcode_${VERSION}_${os}_${arch}"
  do
    url="https://github.com/${REPO}/releases/download/v${VERSION}/${asset}"
    echo "Trying ${url}"
    if curl -fsSL "$url" -o "$DEST"; then
      chmod +x "$DEST"
      return 0
    fi
  done
  return 1
}

build_from_source() {
  if ! command -v go >/dev/null 2>&1; then
    echo "Go not found — install from https://go.dev/dl/ or wait for a release binary." >&2
    return 1
  fi
  echo "Building from source with Go…"
  local tmp
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT
  git clone --depth 1 "https://github.com/${REPO}.git" "$tmp/myagent"
  (cd "$tmp/myagent" && go build -trimpath -ldflags "-s -w -X main.version=${VERSION} -X main.appName=${NAME}" -o "$DEST" ./cmd/localcode)
  chmod +x "$DEST"
}

# Prefer release binary; fall back to source build when run from a checkout.
if [[ -f "$(dirname "$0")/../go.mod" ]]; then
  ROOT="$(cd "$(dirname "$0")/.." && pwd)"
  echo "Building from local checkout…"
  if ! command -v go >/dev/null 2>&1; then
    echo "Go is required to build from source." >&2
    exit 1
  fi
  go build -trimpath -ldflags "-s -w -X main.version=${VERSION} -X main.appName=${NAME}" \
    -o "$DEST" "${ROOT}/cmd/localcode"
  chmod +x "$DEST"
elif download_release; then
  echo "Installed release binary."
elif build_from_source; then
  echo "Installed from source."
else
  echo "Install failed." >&2
  exit 1
fi

# Ensure PATH for future shells
ensure='export PATH="$HOME/.local/bin:$PATH"'
for rc in "${HOME}/.bashrc" "${HOME}/.zshrc"; do
  if [[ -f "$rc" ]] && ! grep -qF '.local/bin' "$rc" 2>/dev/null; then
    printf '\n# myagent\n%s\n' "$ensure" >> "$rc"
  fi
done

echo
echo "Installed: $DEST"
echo "Start the terminal UI:"
echo "  $NAME"
echo
if ! command -v "$NAME" >/dev/null 2>&1; then
  echo "Open a new terminal, or run:  export PATH=\"\$HOME/.local/bin:\$PATH\""
fi
