#!/usr/bin/env bash
# build.sh — Build the entire Rho toolchain and package it for standalone
# deployment. After this script, `./deploy/` contains everything needed on a
# machine without Go or the source tree.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
DEPLOY="${ROOT}/deploy"
BIN="${DEPLOY}/bin"
STD="${DEPLOY}/std"
DOCS="${DEPLOY}/docs"
MODULES="${DEPLOY}/modules/components"

rm -rf "${DEPLOY}"
mkdir -p "${BIN}" "${STD}" "${DOCS}" "${MODULES}/installed/component"

echo "==> Building core toolchain..."
go build -o "${BIN}/rho"      "${ROOT}/cmd/rho"
go build -o "${BIN}/rho-deps" "${ROOT}/cmd/rho-deps"
go build -o "${BIN}/rho-install" "${ROOT}/cmd/rho-install"

echo "==> Building rho-lsp (uses core compiler, must be built inside the repo)..."
go build -o "${BIN}/rho-lsp"  "${ROOT}/modules/components/rho-lsp/src"

echo "==> Building rho-flash (pure Go, reads target specs through rho-target)..."
go build -o "${BIN}/rho-flash" "${ROOT}/modules/components/rho-flash/src"

echo "==> Installing standard library..."
cp -r "${ROOT}/std/." "${STD}/"

echo "==> Installing documentation..."
cp -r "${ROOT}/docs/." "${DOCS}/"

echo "==> Carrying the manifests of the components that ship with the toolchain..."
# The manifest travels so `rho install` can register the component; the binary
# stays in bin/, which is where an install looks for the toolchain's executables.
for component in rho-lsp rho-flash; do
  mkdir -p "${MODULES}/components/${component}"
  cp "${ROOT}/modules/components/${component}/module.yml" "${MODULES}/components/${component}/"
done

echo "==> Creating install manifest..."
cat > "${DEPLOY}/install-manifest.txt" <<'MANIFEST'
rho          — Core compiler/CLI
rho-deps     — Dependency manager
rho-install  — Standalone installer
rho-lsp      — Language server (LSP)
rho-flash    — Device flashing, driven by a board's target spec
std/         — Standard library (.rho sources)
docs/        — Documentation
modules/     — Pre-registered components
MANIFEST

echo ""
echo "Done. Standalone deployment is in: ${DEPLOY}"
echo "Size: $(du -sh "${DEPLOY}" | cut -f1)"
echo ""
echo "To deploy on a target machine:"
echo "  scp -r ${DEPLOY} user@host:/tmp/rho-deploy"
echo "  ssh user@host 'cd /tmp/rho-deploy && ./bin/rho-install --install'"
