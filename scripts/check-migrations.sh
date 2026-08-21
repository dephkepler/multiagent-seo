#!/usr/bin/env bash
# Refuses a migration whose number is not above every migration already on the
# main branch.
#
# golang-migrate stores one version number and only ever moves forward, so a
# migration that reaches production with a LOWER number than one already applied
# there is skipped — silently, with the app logging "migrations up to date" and
# the tables simply absent. That happened: 000048_finance was written as 000045
# while 000046 and 000047 were already merged and deployed, and the deploy came
# up green with no finance schema at all. Nothing in the test suite could see it,
# because the integration harness applies every *.up.sql by filename instead of
# tracking a version.
#
# Also catches two migrations sharing a number, which the same mechanism turns
# into "one of these will never run".
set -euo pipefail

cd "$(dirname "$0")/.."
dir=backend/migrations

version_of() { basename "$1" | cut -d_ -f1; }

# Duplicate numbers are wrong regardless of any remote being reachable.
dupes=$(ls "$dir"/*.up.sql | while read -r f; do version_of "$f"; done | sort | uniq -d)
if [ -n "$dupes" ]; then
	echo "migrations: two files share a version number — one of them will never run:" >&2
	for v in $dupes; do ls "$dir"/${v}_*.up.sql >&2; done
	exit 1
fi

# The baseline is whatever main already carries. Without it (shallow clone, no
# remote, fresh CI) there is nothing to compare against — skip rather than fail.
ref=""
for candidate in github/main origin/main main; do
	if git rev-parse --verify --quiet "$candidate" >/dev/null; then
		ref="$candidate"
		break
	fi
done
if [ -z "$ref" ]; then
	echo "migrations: no main ref to compare against, skipping the ordering check"
	exit 0
fi

merged_max=$(git ls-tree --name-only "$ref" "$dir/" | grep '\.up\.sql$' | while read -r f; do version_of "$f"; done | sort -n | tail -1)
if [ -z "$merged_max" ]; then
	exit 0
fi

status=0
for f in "$dir"/*.up.sql; do
	# only files that main does not have yet
	if git ls-tree --name-only "$ref" "$dir/" | grep -qxF "$f"; then
		continue
	fi
	v=$(version_of "$f")
	if [ "$((10#$v))" -le "$((10#$merged_max))" ]; then
		echo "migrations: $f is numbered $v, but $ref already has $merged_max — it would be skipped on a deployed database. Renumber it above $merged_max." >&2
		status=1
	fi
done
exit $status
