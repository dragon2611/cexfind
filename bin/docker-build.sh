#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
output_dir="${repo_root}/deploy"
mkdir -p "${output_dir}"

build_args=(--target all-binaries --output "type=local,dest=${output_dir}")
if [[ -n "${DOCKER_PLATFORM:-}" ]]; then
    build_args+=(--platform "${DOCKER_PLATFORM}")
fi

docker build "${build_args[@]}" "${repo_root}"
echo "Built ${output_dir}/{webserver,cli,console} (Linux)"
