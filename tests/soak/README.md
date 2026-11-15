# Soak tests

Two programs run for 72 hours while their resident memory is sampled every
minute. Beta needs both to be flat.

| Run | What runs | What it exercises |
|---|---|---|
| `firmware` | [firmware/](firmware), a C program shaped like a sensor node's main loop | strings built and dropped every iteration, the latest value kept in a variable outside the loop, a data enum, a closure capturing a string, `append` into a fixed array, `sort` and `max` |
| `service` | the documentation site (`applications/docsite`), a Go service | about ten requests a second: the overview, the release notes, a page, a miss |

```sh
tests/soak/soak.sh firmware        # 72 hours
tests/soak/soak.sh service 72
RHO=./bin/rho tests/soak/soak.sh firmware 1   # an hour, with a toolchain you just built
```

Each run writes `logs/<run>-<start>.csv` (`minute,rss_kb`), the program's
output beside it (`.out`, not kept in git), and a `.verdict` at the end. The
first hour is warm-up. Memory is **flat** when the mean of the last hour is
within 5% of the mean of the first hour after warm-up, and no sample after
warm-up is more than 1.5 times the smallest. A run that is stopped early, or
whose program exits, says so and fails.

`SOAK_INTERVAL` changes the sampling interval (seconds) and `SOAK_PORT` the
service's port (18080).
