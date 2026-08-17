#!/usr/bin/env bash
# Runs "$@" only if no other instance holds the named lock — used to stop
# concurrent `make dev` / `make deploy` runs (e.g. two Claude Code agents,
# local + a teammate, or a stray background run) from fighting over the same
# ports / docker compose project / air-managed process.
#
# On conflict this FAILS FAST (no retry, no wait) — the caller must not hang;
# it should surface the message and stop. This is deliberate: a build lock or
# a bound port would otherwise make the second run sit for minutes looking
# like a hang, which is exactly the problem this script exists to avoid.
#
# Portable (mkdir is atomic on every POSIX fs — no flock dependency needed,
# since this also has to run on macOS where flock(1) isn't installed by
# default). Locks live in /tmp, so a host reboot always clears stale ones.
#
# Usage: LOCK_NAME=deploy scripts/with-lock.sh <command> [args...]
set -euo pipefail

name="${LOCK_NAME:?LOCK_NAME env var required, e.g. LOCK_NAME=deploy}"
lockdir="/tmp/multiagent-seo-${name}.lock.d"

if ! mkdir "$lockdir" 2>/dev/null; then
    holder_pid="$(cat "$lockdir/pid" 2>/dev/null || echo unknown)"
    mtime=$(stat -f %m "$lockdir" 2>/dev/null || stat -c %Y "$lockdir" 2>/dev/null || echo 0)
    age=$(( $(date +%s) - mtime ))
    echo "✋ '$name' is already running (pid $holder_pid, lock held ${age}s: $lockdir) — aborting, not waiting." >&2
    echo "   Do NOT retry in a loop and do NOT remove the lock yourself unless you have confirmed with the user that the holder is actually dead (e.g. that pid no longer exists)." >&2
    echo "   Stale-lock cleanup (only after confirming): rm -rf $lockdir" >&2
    exit 1
fi
echo $$ > "$lockdir/pid"
trap 'rm -rf "$lockdir"' EXIT

"$@"
