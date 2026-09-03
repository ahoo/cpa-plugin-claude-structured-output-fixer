#!/usr/bin/env bash
# Build claude-structured-output-fixer for the CLIProxyAPI native plugin host.
set -euo pipefail

SRC_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PLUGIN_NAME="claude-structured-output-fixer"
PLUGIN_VERSION="0.1.0"
OUT_DIR="${PLUGIN_OUT_DIR:-${SRC_DIR}/../../plugins/linux/amd64}"

# The runtime image uses glibc, so build with a Debian toolchain rather than musl.
GO_IMAGE="${PLUGIN_GO_IMAGE:-golang:1.26-bookworm}"

docker run --rm \
  -v "${SRC_DIR}:/src:ro" \
  -v "${OUT_DIR}:/out" \
  -w /src \
  -e "CGO_ENABLED=1" \
  "${GO_IMAGE}" \
  sh -ec '
    go build -buildmode=c-shared -o /out/'"${PLUGIN_NAME}"'-v'"${PLUGIN_VERSION}"'.so .
    rm -f /out/'"${PLUGIN_NAME}"'-v'"${PLUGIN_VERSION}"'.h
  '

printf '[build] ok: %s/%s-v%s.so\n' "${OUT_DIR}" "${PLUGIN_NAME}" "${PLUGIN_VERSION}"
