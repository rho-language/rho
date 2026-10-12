#!/usr/bin/env bash
# check.sh — Build this repository's modules, put the components and plugins it
# carries into the local registry, install the toolchain, and run the tests.
#
# The order is the whole of it. A component is a separate Go module that the
# toolchain runs as a program, so a test that builds a program is a test that
# needs rho-target, rho-cimport and rho-kconfig present in the local registry and
# enabled. Running `go test ./...` before that does not fail because anything is
# broken; it fails with "this build needs the rho-target component: module not
# installed", which is a true statement about the environment and a useless one
# about the code.
#
# So the procedure is: build, publish, install, test — in that order, every time,
# by this script or by the four commands it prints. It is the same procedure on a
# machine that has never run it, which is what makes it usable on CI unchanged.
#
#   ./check.sh                     build and publish everything, install, test
#   ./check.sh publish NAME...     build and publish only those, then stop
#
# `publish` is for an image that needs a few components rather than all of them:
# the registry service names six, and hand-writing their build-and-publish steps
# in a Dockerfile is a list that stops agreeing with the tree.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
cd "${ROOT}"

BIN="${ROOT}/bin"

# every_module prints the modules this repository carries, one per line.
every_module() {
  local module
  for module in modules/components/*/ modules/plugins/*/; do
    [ -f "${module}go.mod" ] || continue
    printf '%s\n' "${module}"
  done
}

# module_named is the directory of one module by its name, or nothing.
module_named() {
  local wanted="$1" module
  for module in $(every_module); do
    [ "$(basename "${module}")" = "${wanted}" ] && { printf '%s\n' "${module}"; return 0; }
  done
  return 1
}

# build_module builds one module's binary, named by `main:` in its manifest.
#
# Not `go build ./...`: that writes a binary per main package named after the
# package, and a component's main package is called `src` — which is a directory
# this script is standing in. The failure is `build output "src" already exists
# and is a directory`, which is a complaint about the build system rather than
# about anything a reader would connect to the component.
build_module() {
  local module="$1" main
  main="$(sed -n 's/^main:[[:space:]]*//p' "${module}module.yml" | head -1)"
  [ -n "${main}" ] || return 0
  (cd "${module}" && go build -o "${main}" ./src)
}

# publish_module publishes one module at the version its own manifest declares,
# and enables it.
#
# The version comes from the module rather than from here, so a component that has
# been version-bumped is published at its new version and there is no list here to
# update and no version here to get wrong. Enabling it is part of publishing rather
# than a step after it, because a component installed but not enabled produces
# "module not installed" — which points at the registry rather than at the switch
# that was not thrown.
publish_module() {
  local module="$1" name version
  name="$(basename "${module}")"
  echo "--> ${name}"
  "${BIN}/rho" module publish "${module}"
  version="$(sed -n 's/^  version:[[:space:]]*//p' "${module}module.yml" | head -1)"
  if [ -n "${version}" ]; then
    "${BIN}/rho" module enable "${name}@${version}" >/dev/null 2>&1 || true
  fi
}

# ---------------------------------------------------------------------------
#  publish: build and publish the named modules, then stop.
# ---------------------------------------------------------------------------
if [ "${1:-}" = "publish" ]; then
  shift
  echo "==> Building the core toolchain..."
  go build -o "${BIN}/rho" "${ROOT}/cmd/rho"

  # Publishing writes into the registry, so the registry has to be there first.
  # The Dockerfile installs the toolchain before this runs, which is why a caller
  # that has not needs it here.
  "${BIN}/rho" install --update

  echo "==> Building and publishing the named components..."
  for wanted in "$@"; do
    module="$(module_named "${wanted}")" || {
      echo "check.sh: this tree carries no module named ${wanted}" >&2
      exit 2
    }
    build_module "${module}"
    publish_module "${module}"
  done
  exit 0
fi

# ---------------------------------------------------------------------------
#  The whole procedure.
# ---------------------------------------------------------------------------
echo "==> Building the core toolchain..."
go build -o "${BIN}/rho"      "${ROOT}/cmd/rho"
go build -o "${BIN}/rho-deps" "${ROOT}/cmd/rho-deps"

echo "==> Installing the toolchain (PATH, the std library, the docs)..."
#
# Before publishing anything, because publishing writes into the local registry
# and it is this that installs the registry. A machine that has never run this
# script has no registry at all, and the order that looks natural — build, publish,
# install, test — stops at the first publish with "the local registry is not
# installed", which says where the fix is not.
#
# --update rather than --force: this refreshes what an install carries without
# emptying the registry first, which is what makes it safe to run on a machine
# that has components installed which this tree does not carry. --force is the one
# destructive step here and stays a thing a person does on purpose.
"${BIN}/rho" install --update

echo "==> Building each component's binary..."
for module in $(every_module); do
  build_module "${module}"
done

echo "==> Publishing the components and plugins this tree carries..."
for module in $(every_module); do
  publish_module "${module}"
done

echo "==> Testing the core module..."
go test ./...

echo "==> Testing every module in the repository..."
#
# Each in its own module, because a component is a separate Go module and `go
# test ./...` from the root does not reach into one. `go vet` and `go test` both
# compile and neither leaves a binary behind, so this checks the whole module
# without producing artefacts in it.
for module in $(every_module); do
  echo "--> $(basename "${module}")"
  (cd "${module}" && go vet ./... && go test ./...)
done

echo "==> Done. The toolchain is at ${BIN}/rho and is on PATH."