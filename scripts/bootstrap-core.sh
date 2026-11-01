#!/usr/bin/env bash
# Restore the `core` symlink that points at the sibling GUI checkout.
#
# The committed go.work resolves the CLI, the GUI and the shared core without
# the symlink, but tools that build a single module in isolation (and CI jobs
# that predate the workspace) still expect ./core to exist. Run this after a
# fresh clone, or let `make bootstrap` run it.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
core_dir="$root/../mod-downloader-gui-v3/core"
link="$root/core"

if [[ ! -d "$core_dir" ]]; then
  echo "error: expected the GUI checkout at $core_dir" >&2
  echo "clone it next to this repository, for example:" >&2
  echo "  git clone <gui-repo> \"$root/../mod-downloader-gui-v3\"" >&2
  exit 1
fi

if [[ -L "$link" ]]; then
  if [[ "$(readlink "$link")" == "../mod-downloader-gui-v3/core" ]]; then
    echo "core symlink already points at ../mod-downloader-gui-v3/core"
    exit 0
  fi
  echo "error: $link points at $(readlink "$link"); remove it first" >&2
  exit 1
fi

if [[ -e "$link" ]]; then
  echo "error: $link exists and is not a symlink; remove it first" >&2
  exit 1
fi

ln -s ../mod-downloader-gui-v3/core "$link"
echo "linked core -> ../mod-downloader-gui-v3/core"

if [[ "${OSTYPE:-}" == msys* || "${OSTYPE:-}" == cygwin* || "${OSTYPE:-}" == win32* ]]; then
  echo "note: on Windows enable symlink support with: git config --global core.symlinks true"
fi
