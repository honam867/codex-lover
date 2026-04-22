#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
bin_dir="${HOME}/.local/bin"
target="${bin_dir}/codex-lover"
bashrc_path="${HOME}/.bashrc"
hook_start="# >>> codex-lover auto-start >>>"
hook_end="# <<< codex-lover auto-start <<<"

go_bin="${GO_BIN:-}"
if [[ -z "${go_bin}" ]]; then
  if command -v go >/dev/null 2>&1; then
    go_bin="$(command -v go)"
  elif [[ -x "${HOME}/.local/bin/go" ]]; then
    go_bin="${HOME}/.local/bin/go"
  fi
fi

if [[ -z "${go_bin}" ]]; then
  echo "Go is required but was not found in PATH." >&2
  echo >&2
  echo "Install one of these first:" >&2
  echo "  sudo apt update && sudo apt install -y golang-go" >&2
  echo "  or install a local Go toolchain under ~/.local/bin/go" >&2
  exit 1
fi

mkdir -p "${bin_dir}"

echo "Building codex-lover..."
(cd "${repo_root}" && "${go_bin}" build -o "${target}" ./cmd/codex-lover)

if [[ -f "${bashrc_path}" ]] && ! grep -Fq "${hook_start}" "${bashrc_path}"; then
  cat >> "${bashrc_path}" <<'EOF'

# >>> codex-lover auto-start >>>
if [[ $- == *i* ]] && command -v codex-lover >/dev/null 2>&1; then
  codex-lover server start >/dev/null 2>&1 || true
fi
# <<< codex-lover auto-start <<<
EOF
fi

"${target}" server start >/dev/null 2>&1 || true

echo
echo "Installed: ${target}"
echo "Make sure ${bin_dir} is in PATH."
echo "Daemon log: ${HOME}/.codex-lover/logs/server.log"
echo
echo "Next steps:"
echo "  codex-lover server status"
echo "  codex-lover status"
echo "  codex-lover watch"
