#!/usr/bin/env bash
#
# @file soak.sh
# @brief Run a program for hours and say whether its memory stayed flat.
#
#   tests/soak/soak.sh firmware [hours]   the C loop in tests/soak/firmware
#   tests/soak/soak.sh service  [hours]   the docsite, a Go service, under load
#
# The program's resident memory is sampled every minute into
# tests/soak/logs/<name>-<start>.csv. At the end — or when stopped with Ctrl-C —
# the first hour is taken as warm-up and the verdict compares the rest: memory
# is flat when the mean of the last hour is within 5% of the mean of the first
# hour after warm-up, and no sample after warm-up is more than 1.5 times the
# smallest. Short runs (under three hours) compare the first and last thirds
# after a tenth of the run as warm-up. The default is 72 hours.
#
# SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
#
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
root="$(cd "$here/../.." && pwd)"
kind="${1:-}"
hours="${2:-72}"
interval="${SOAK_INTERVAL:-60}"
rho="${RHO:-rho}"
case "$kind" in
  firmware|service) ;;
  *) echo "usage: $0 firmware|service [hours]" >&2; exit 2 ;;
esac

mkdir -p "$here/logs"
start="$(date -u +%Y%m%dT%H%M%SZ)"
log="$here/logs/$kind-$start.csv"
pids=()
cleanup() { for p in "${pids[@]}"; do kill "$p" 2>/dev/null || true; done; }
trap cleanup EXIT

if [ "$kind" = firmware ]; then
  (cd "$here/firmware" && "$rho" build . >/dev/null)
  "$here/firmware/build/output/soak" > "$here/logs/$kind-$start.out" &
  target=$!; pids+=("$target")
else
  port="${SOAK_PORT:-28411}"
  (cd "$root" && "$rho" build applications/docsite >/dev/null)
  (cd "$root" && RHO_HTTP_ADDR="127.0.0.1:$port" RHO_DOCS_ROOT=. RHO_DOCS_TREE=docs \
     RHO_SITE_RELEASES=CHANGELOG.md exec ./applications/docsite/build/docsite) \
     > "$here/logs/$kind-$start.out" 2>&1 &
  target=$!; pids+=("$target")
  sleep 3
  # A steady load: the overview, the release notes, a page and a miss, about
  # ten requests a second.
  (while kill -0 "$target" 2>/dev/null; do
     for path in / /releases /healthz /docs /docs/language/syntax /missing; do
       curl -s -o /dev/null "http://127.0.0.1:$port$path" || true
     done
     sleep 0.5
   done) &
  pids+=("$!")
fi

echo "minute,rss_kb" > "$log"
echo "soak $kind: pid $target, $hours h, sampling every ${interval}s into $log"
end=$(( $(date +%s) + $(awk "BEGIN{printf \"%d\", $hours*3600}") ))
minute=0
verdict() {
  awk -F, -v hours="$hours" 'NR>1 {m[NR-2]=$2; n=NR-1}
    END {
      if (n < 6) { print "too few samples (" n ") for a verdict"; exit 1 }
      per_hour = n / hours; warm = int(per_hour); if (hours < 3) warm = int(n/10)
      if (hours < 3) { span = int((n-warm)/3) } else { span = int(per_hour) }
      if (span < 1) span = 1
      lo = 1e18; hi = 0
      for (i = warm; i < n; i++) { if (m[i] < lo) lo = m[i]; if (m[i] > hi) hi = m[i] }
      a = 0; for (i = warm; i < warm+span; i++) a += m[i]; a /= span
      b = 0; for (i = n-span; i < n; i++) b += m[i]; b /= span
      growth = (b - a) / a * 100
      printf "samples %d, after warm-up min %d kB max %d kB; first %.0f kB, last %.0f kB, growth %.2f%%\n", n, lo, hi, a, b, growth
      if (growth > 5 || hi > 1.5*lo) { print "NOT FLAT"; exit 1 }
      print "FLAT"
    }' "$log"
}
while [ "$(date +%s)" -lt "$end" ] && kill -0 "$target" 2>/dev/null; do
  rss=$(awk '/VmRSS/ {print $2}' "/proc/$target/status" 2>/dev/null || echo 0)
  echo "$minute,$rss" >> "$log"
  minute=$((minute+1))
  sleep "$interval"
done
if ! kill -0 "$target" 2>/dev/null; then
  echo "soak $kind: the program stopped before the end (see ${log%.csv}.out)"
  exit 1
fi
verdict | tee "${log%.csv}.verdict"
