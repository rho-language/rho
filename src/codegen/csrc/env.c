#include <stdlib.h>
/*
 * env.c — the environment intrinsic.
 *
 * Separate from strings.c because a program that never asks about its environment
 * should not carry this one, and on a board there is no environment to ask about.
 *
 * SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
 */

const char* rho_env_or(const char* name, const char* fallback)
{
    const char* value = name != NULL ? getenv(name) : NULL;
    return value != NULL ? value : fallback;
}


/* rho_getenv reads the value of an environment variable, or "" when it is not
 * set. The same answer the Go runtime gives, so a program means the same thing
 * on either backend. */
char* rho_getenv(const char* name)
{
    const char* value = getenv(name);
    /* A copy in the string arena: the environment's own storage changes with
     * the next setenv, and a string a program holds must not. */
    return rho_str_dup(value);
}

/* rho_setenv sets a variable for this process and the programs it runs,
 * replacing a value already set, as the Go runtime's does. */
void rho_setenv(const char* name, const char* value)
{
    if (name != NULL && value != NULL) {
        setenv(name, value, 1);
    }
}
