#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
case "${1:-$(uname -m)}" in
    arm64|aarch64) arch=arm64 ;;
    amd64|x86_64) arch=amd64 ;;
    *) echo "unsupported Mac architecture: ${1:-$(uname -m)}" >&2; exit 1 ;;
esac

output_dir="${repo_root}/deploy/macos-${arch}"
mkdir -p "${output_dir}"

docker build --platform "linux/${arch}" --build-arg BUILD_OS=darwin \
    --target all-binaries --output "type=local,dest=${output_dir}" "${repo_root}"
echo "Built ${output_dir}/{webserver,cli,console} (macOS ${arch})"
