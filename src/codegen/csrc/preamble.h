/*
 * preamble.h — what every generated Rho C file starts with.
 *
 * This is a real C header rather than Go strings so that it can be read, linted,
 * formatted and navigated as C: an editor gives it syntax highlighting, clang-format
 * rewrites it, and a static analyser checks it. The alternative — the same text
 * assembled from `WriteString` calls in Go — has none of those, and the escaping
 * is where the mistakes were.
 *
 * `{{.Features}}` is the only substitution; everything else is C.
 *
 * No include guard, deliberately. This is a preamble rather than a header: it is
 * emitted into each generated file that needs it, and a guard would silently drop
 * it from every file but the first — which is how the weak `_init` and `_fini`
 * stopped being defined on one side of a two-file project and the link failed
 * naming a symbol rather than anything that pointed at the guard.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

#define _POSIX_C_SOURCE 200809L

#include <stdint.h>
#include <stdbool.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stddef.h>

/* Every `print` in a program becomes one of these.
 *
 * Declared before the program's own functions and defined in the runtime, because
 * a function that prints has to know it exists before it is read. Without the
 * prototype the C compiler assumes an implicit `int rho_print(const char *)`,
 * which is a different function with a different calling convention.
 */
void rho_print(const char* s);

{{range .Features}}#define RHO_FEATURE_{{.}} 1
{{end}}
