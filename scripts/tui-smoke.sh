#!/usr/bin/env bash
# Real-terminal smoke test for the tview client.
#
# Builds the CLI, starts it inside a detached tmux session (a real PTY), drives
# it with key presses and resizes, and fails if the expected screens never
# appear or the process does not exit on Ctrl+C. Network is required because the
# TUI fetches the Minecraft release manifest on startup.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP="$(mktemp -d)"
SESSION="moddl-smoke-$$"
KEEP="${MOD_DOWNLOADER_KEEP_SMOKE:-0}"

cleanup() {
    tmux kill-session -t "$SESSION" 2>/dev/null || true
    if [[ "$KEEP" != "1" ]]; then
        rm -rf "$TMP"
    else
        echo "smoke artifacts kept in $TMP"
    fi
}
trap cleanup EXIT

if ! command -v tmux >/dev/null 2>&1; then
    echo "SKIP: tmux is not installed" >&2
    exit 1
fi

WORK="$TMP/work"
HOME_DIR="$TMP/home"
CACHE_DIR="$TMP/cache"
mkdir -p "$WORK" "$HOME_DIR" "$CACHE_DIR"
BIN="$TMP/mod-downloader-cli"

echo "==> building $BIN"
(cd "$ROOT" && go build -o "$BIN" ./cmd/mod-downloader-cli)

capture() {
    tmux capture-pane -p -t "$SESSION" 2>/dev/null || true
}

dump() {
    echo "---- last frame ----" >&2
    capture | tail -n 30 >&2
    echo "--------------------" >&2
}

wait_for() {
    local needle="$1" timeout="${2:-30}" start
    start=$SECONDS
    while (( SECONDS - start < timeout )); do
        if capture | grep -qF -- "$needle"; then
            return 0
        fi
        sleep 0.2
    done
    echo "ERROR: timed out waiting for '$needle'" >&2
    dump
    return 1
}

wait_gone() {
    local needle="$1" timeout="${2:-15}" start
    start=$SECONDS
    while (( SECONDS - start < timeout )); do
        if ! capture | grep -qF -- "$needle"; then
            return 0
        fi
        sleep 0.2
    done
    echo "ERROR: '$needle' stayed on screen" >&2
    dump
    return 1
}

send() {
    tmux send-keys -t "$SESSION" "$@"
}

echo "==> starting detached tmux session $SESSION"
tmux new-session -d -s "$SESSION" -x 100 -y 30 -c "$WORK" \
    -e HOME="$HOME_DIR" \
    -e XDG_CACHE_HOME="$CACHE_DIR" \
    -e LOCALAPPDATA="$CACHE_DIR" \
    -e TMPDIR="$TMP" \
    "$BIN"

wait_for "Application status and history" 60

echo "==> keyboard navigation"
send "3"
wait_for "Local Mod Management" 15
send "5"
wait_for "Pinned Mod Versions" 15
send "6"
wait_for "Settings" 15

echo "==> help dialog and escape layering"
send -l "?"
wait_for "Keyboard shortcuts" 15
send Escape
wait_gone "Keyboard shortcuts" 15

echo "==> resize while running"
tmux resize-window -t "$SESSION" -x 60 -y 20
wait_for "Settings" 15
if capture | awk 'length($0) > 60 { exit 1 }'; then
    :
else
    echo "ERROR: frame wider than the resized terminal" >&2
    dump
    exit 1
fi
tmux resize-window -t "$SESSION" -x 120 -y 40
wait_for "Settings" 15

echo "==> quitting with Ctrl+C"
send C-c
for _ in $(seq 1 100); do
    if ! tmux has-session -t "$SESSION" 2>/dev/null; then
        break
    fi
    sleep 0.2
done
if tmux has-session -t "$SESSION" 2>/dev/null; then
    echo "ERROR: TUI did not exit after Ctrl+C" >&2
    dump
    exit 1
fi

echo "OK: real-terminal smoke test passed"
