#!/usr/bin/env bash
set -euo pipefail

VERSION="${1:-dev}"
OUT_DIR="${2:-dist}"
PKG="./cmd/ntp-server"

mkdir -p "${OUT_DIR}"

targets=(
  "linux amd64"
  "linux arm64"
  "darwin amd64"
  "darwin arm64"
  "windows amd64"
)

for target in "${targets[@]}"; do
  IFS=' ' read -r os arch <<< "${target}"
  name="ntp-server-${os}-${arch}"
  if [[ "${os}" == "windows" ]]; then
    name="${name}.exe"
  fi

  echo "Building ${name}"
  CGO_ENABLED=0 GOOS="${os}" GOARCH="${arch}" \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o "${OUT_DIR}/${name}" "${PKG}"
done

echo "Build artifacts are in ${OUT_DIR}/"
