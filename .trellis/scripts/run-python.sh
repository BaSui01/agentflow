#!/usr/bin/env bash
set -euo pipefail

PYENV_ROOT="${PYENV_ROOT:-$HOME/.pyenv}"
PYENV_BIN="$PYENV_ROOT/bin/pyenv"

if [ -x "$PYENV_BIN" ]; then
  export PYENV_ROOT
  export PATH="$PYENV_ROOT/bin:$PYENV_ROOT/shims:$PATH"
  exec "$PYENV_BIN" exec python3 "$@"
fi

if command -v python3 >/dev/null 2>&1; then
  exec python3 "$@"
fi

exec python "$@"
