#!/usr/bin/env bash
# check-local-paths.sh — fail when a tracked file names a place in a particular
# person's home. A manifest that has to reach a sibling checkout says
# `${RHO_WORKSPACE}/name` (or its ecosystem's own `${X_WORKSPACE}`); a path under
# someone's home directory is true for one machine and is published to all.
#
# What counts is the home of whoever runs it, any more named in LOCAL_PATH_USERS
# (space separated), and by shape anyone's workspace: `/home/<user>/projects/` and
# `/Users/<user>/`. A fixed path that is the *target's* — the `/home/arduino/apps`
# a board is deployed to, a container's `/home/rho` — is a fact about that target,
# not a leak, and is not matched.
#
#   scripts/check-local-paths.sh [repository...]     default: the current one
#   LOCAL_PATH_USERS="alice bob" scripts/check-local-paths.sh ../app
set -euo pipefail
homes=("$HOME")
for user in ${LOCAL_PATH_USERS:-}; do homes+=("/home/$user" "/Users/$user"); done
pattern=$(printf '%s\n' "${homes[@]}" | sed 's/[][\.*^$/]/\\&/g' | paste -sd'|')
pattern="${pattern}|/home/[A-Za-z0-9_.-]+/projects|/Users/[A-Za-z0-9_.-]+"
status=0
for repo in "${@:-.}"; do
  hits=$(git -C "$repo" grep -nIE "(${pattern})(/|\$|[^A-Za-z0-9_.-])" -- . ':!*/testdata/*' || true)
  if [ -n "$hits" ]; then
    echo "local paths in $repo:" >&2
    echo "$hits" | sed 's/^/  /' | cut -c1-200 >&2
    status=1
  fi
done
exit $status
