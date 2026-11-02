#!/usr/bin/env bash
set -euo pipefail

IDENA_GO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GO_TOOLCHAIN="${IDENA_GO_GOTOOLCHAIN:-go1.27.3}"

if ! command -v go >/dev/null 2>&1; then
  echo "Go toolchain is missing." >&2
  exit 1
fi

if [[ "${IDENA_GO_DIR}" =~ [[:space:]] ]]; then
  WORKSPACE_DIR="$(cd "${IDENA_GO_DIR}/.." && pwd)"
  workspace_id="$(printf '%s' "${WORKSPACE_DIR}" | cksum | awk '{print $1}')"
  LINK_DIR="${TMPDIR:-/tmp}/idena-clean-fork-${UID:-user}-${workspace_id}"

  if [[ -L "${LINK_DIR}" ]]; then
    if [[ "$(readlink "${LINK_DIR}")" != "${WORKSPACE_DIR}" ]]; then
      echo "Whitespace-free Go workspace symlink points to an unexpected path: ${LINK_DIR}" >&2
      exit 1
    fi
  elif [[ -e "${LINK_DIR}" ]]; then
    echo "Cannot create whitespace-free Go workspace symlink at ${LINK_DIR}; path exists and is not a symlink." >&2
    exit 1
  elif ! ln -s "${WORKSPACE_DIR}" "${LINK_DIR}"; then
    # Another invocation may have created the same workspace-specific link.
    if [[ ! -L "${LINK_DIR}" || "$(readlink "${LINK_DIR}")" != "${WORKSPACE_DIR}" ]]; then
      echo "Cannot create whitespace-free Go workspace symlink at ${LINK_DIR}." >&2
      exit 1
    fi
  fi

  IDENA_GO_DIR="${LINK_DIR}/$(basename "${IDENA_GO_DIR}")"
fi

cd "${IDENA_GO_DIR}"
exec env GOTOOLCHAIN="${GO_TOOLCHAIN}" go "$@"
