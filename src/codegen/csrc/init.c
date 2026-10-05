/*
 * init.c — the hooks a C library calls around a program's own initialisers.
 *
 * `_init` and `_fini` are the C library's hooks for code that runs before and
 * after everything else. A board's startup assembly calls the library's
 * initializer, which calls them, and a build with `-nostartfiles` — which a
 * bare-metal build has, so that its own startup is the entry point rather than the
 * compiler's — has excluded the two files that define them.
 *
 * Weak, because the same generated project is also what a hosted target builds,
 * and there the C library defines them properly. A strong definition would collide
 * with it; a weak one yields to it and is used only where nothing else provides
 * them, which is exactly the bare-metal case and exactly when they are needed.
 *
 * Empty, which is what they are for on a board: there is nothing to do before a
 * program's own initialisers, and the library's own work is done by
 * `__libc_init_array`, which the startup calls separately.
 *
 * Always emitted, and in the runtime rather than in the program, for two reasons.
 * A C project has no generated program for this to live in. And the reference that
 * needs them comes from the C library's initializer, which is in a different
 * archive — so nothing in either generated file refers to them, and a fragment
 * gated on what the program calls would leave them out of a program that cannot
 * link without them.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

__attribute__((weak)) void _init(void);
__attribute__((weak)) void _init(void) {}

__attribute__((weak)) void _fini(void);
__attribute__((weak)) void _fini(void) {}
