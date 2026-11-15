#!/bin/sh
# Builds the C and the Go program and checks that both print the same thing,
# and exit with the same status, for the sample and for each malformed input.
# `rho test` replaces build/csv_report with a test program (see FINDINGS.md),
# so the programs are built again here first.
set -u
cd "$(dirname "$0")/.." || exit 1
rho build . >/dev/null 2>&1 || { echo "rho build failed"; exit 1; }
go_bin=build/csv_report
c_bin=build/output/csv_report
tmp=build/parity
mkdir -p "$tmp"
failed=0
n=0
check() {
    n=$((n + 1))
    "$go_bin" "$@" >"$tmp/go.txt" 2>&1; echo "exit $?" >>"$tmp/go.txt"
    "$c_bin" "$@" >"$tmp/c.txt" 2>&1; echo "exit $?" >>"$tmp/c.txt"
    if cmp -s "$tmp/go.txt" "$tmp/c.txt"; then
        echo "ok $n - $*"
    else
        echo "not ok $n - $*"
        diff "$tmp/go.txt" "$tmp/c.txt"
        failed=1
    fi
}
check sample.csv
check sample.csv --group-by region --sum units
check sample.csv --group-by customer --sum price
check sample.csv --group-by product
check tests/data/unterminated.csv
check tests/data/short_row.csv
check tests/data/bad_number.csv --group-by k --sum v
check tests/data/empty.csv
check missing.csv
check sample.csv --sum price
exit $failed
