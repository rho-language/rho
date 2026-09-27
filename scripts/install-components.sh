#!/usr/bin/env bash
#
# Publish and install the in-tree Rho components.
#
# A component is built like any other module: `rho module publish` packs it,
# installs it under ~/.rho/modules/installed and records it in the registry, so
# `rho fmt`, `rho log`, `rho test` and `rho auth` are dispatched to it. There is
# nothing to build in the core toolchain for these.
#
# Usage:
#   scripts/install-components.sh            # publish every component
#   scripts/install-components.sh rho-log    # publish one
#
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
rho="${RHO_BIN:-$root/bin/rho}"

if [[ ! -x "$rho" ]]; then
  echo "no rho at $rho; build it first: go build -o ./bin/rho ./cmd/rho" >&2
  exit 1
fi

# Dependency order: rho-fmt provides the record formatter, and the components
# that log depend on rho-log.
components=("rho-fmt" "rho-log" "rho-test" "rho-auth")
if [[ $# -gt 0 ]]; then
  components=("$@")
fi

for component in "${components[@]}"; do
  dir="$root/modules/components/$component"
  if [[ ! -d "$dir" ]]; then
    echo "no such component: $component" >&2
    exit 1
  fi
  printf '%-9s ' "$component"
  (cd "$dir" && "$rho" module publish >/dev/null)
  echo "published"
done

echo
echo "Installed components:"
"$rho" module list 2>/dev/null | sed 's/^/  /' || true
