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

echo "==> Installing standard library..."
cp -r "${ROOT}/std/." "${STD}/"

echo "==> Installing documentation..."
cp -r "${ROOT}/docs/." "${DOCS}/"

echo "==> Carrying the rho-lsp manifest..."
# The manifest travels so `rho install` can register the component; the binary
# stays in bin/, which is where an install looks for the toolchain's executables.
mkdir -p "${MODULES}/components/rho-lsp"
cp "${ROOT}/modules/components/rho-lsp/module.yml" "${MODULES}/components/rho-lsp/"

echo "==> Creating install manifest..."
cat > "${DEPLOY}/install-manifest.txt" <<'MANIFEST'
rho          — Core compiler/CLI
rho-deps     — Dependency manager
rho-install  — Standalone installer
rho-lsp      — Language server (LSP)
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
