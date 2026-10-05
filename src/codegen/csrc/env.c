/*
 * env.c — the environment intrinsic.
 *
 * Separate from strings.c because a program that never asks about its environment
 * should not carry this one, and on a board there is no environment to ask about.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

static const char* rho_env_or(const char* name, const char* fallback)
{
    const char* value = name != NULL ? getenv(name) : NULL;
    return value != NULL ? value : fallback;
}