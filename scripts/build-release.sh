#!/usr/bin/env bash
# Cross-compile LocalCode for Linux, Windows, and macOS.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="${ROOT}/dist"
VERSION="${VERSION:-0.4.0}"
mkdir -p "$OUT"

export PATH="${HOME}/.local/go/bin:${PATH}"

targets=(
  "linux/amd64"
  "linux/arm64"
  "darwin/amd64"
  "darwin/arm64"
  "windows/amd64"
)

echo "Building myagent ${VERSION}…"
for t in "${targets[@]}"; do
  os="${t%/*}"
  arch="${t#*/}"
  name="myagent_${VERSION}_${os}_${arch}"
  alt="localcode_${VERSION}_${os}_${arch}"
  ext=""
  if [[ "$os" == "windows" ]]; then
    ext=".exe"
  fi
  echo "  -> ${name}${ext}"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION} -X main.appName=myagent" \
    -o "${OUT}/${name}${ext}" "${ROOT}/cmd/localcode"
  # keep localcode-named alias for older install scripts
  cp "${OUT}/${name}${ext}" "${OUT}/${alt}${ext}"
done

(
  cd "$OUT"
  sha256sum myagent_${VERSION}_* localcode_${VERSION}_* > "myagent_${VERSION}_checksums.txt" 2>/dev/null \
    || shasum -a 256 myagent_${VERSION}_* localcode_${VERSION}_* > "myagent_${VERSION}_checksums.txt"
)

echo "Done. Artifacts in ${OUT}"
ls -lh "$OUT"
