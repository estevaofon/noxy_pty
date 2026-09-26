#!/usr/bin/env sh
# Compila todos os binarios em dist/ e o checksums.txt. creack/pty e Go
# puro, entao um runner so faz a matriz inteira com CGO_ENABLED=0.
set -eu
NAME="${1:?usage: build.sh <extension-name>}"
mkdir -p dist
for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64; do
  os=${target%/*}; arch=${target#*/}
  ext=""; [ "$os" = windows ] && ext=".exe"
  GOOS=$os GOARCH=$arch CGO_ENABLED=0 go build -trimpath -ldflags=-s -o "dist/noxy-plugin-$NAME-$os-$arch$ext" .
done
if command -v sha256sum >/dev/null 2>&1; then sum="sha256sum"; else sum="shasum -a 256"; fi
(cd dist && $sum -- noxy-plugin-* > checksums.txt)
echo "dist/ pronto"
