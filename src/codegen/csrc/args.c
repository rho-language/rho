/*
 * args.c — the program's command line.
 *
 * The generated `main` hands its `argc` and `argv` to app_args, which keeps them
 * here; the rest reads them. The answers are the Go runtime's: the program's own
 * name is not an argument, so rho_arg(0) is the first thing written after it,
 * and get_args is every argument joined by one space.
 *
 * A board has no command line: nothing calls rho_args_init, and a program there
 * has no arguments — rho_arg is "" and rho_arg_count 0.
 *
 * SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
 */

static int rho_args_argc;
static char** rho_args_argv;

void rho_args_init(int argc, char** argv)
{
    rho_args_argc = argv != NULL && argc > 0 ? argc : 0;
    rho_args_argv = argv;
}

uint32_t rho_arg_count(void)
{
    return rho_args_argc > 1 ? (uint32_t)(rho_args_argc - 1) : 0u;
}

/* One argument by position from 0, or "" past the last. Borrowed from argv,
 * which lives as long as the program. */
const char* rho_arg(int32_t index)
{
    if (index < 0 || (uint32_t)index >= rho_arg_count()) {
        return "";
    }
    const char* value = rho_args_argv[index + 1];
    return value != NULL ? value : "";
}

/* Every argument, joined by one space, in the string arena. */
char* rho_get_args(void)
{
    uint32_t count = rho_arg_count();
    size_t n = 1;
    for (uint32_t i = 0; i < count; i++) {
        n += strlen(rho_arg((int32_t)i)) + 1;
    }
    char* out = rho_str_alloc(n);
    if (out == NULL) {
        return (char*)"";
    }
    size_t at = 0;
    for (uint32_t i = 0; i < count; i++) {
        const char* value = rho_arg((int32_t)i);
        size_t length = strlen(value);
        if (i > 0) {
            out[at++] = ' ';
        }
        memcpy(out + at, value, length);
        at += length;
    }
    out[at] = 0;
    return out;
}
