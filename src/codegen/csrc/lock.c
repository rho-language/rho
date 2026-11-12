/*
 * lock.c — named locks.
 *
 * A generated C program runs one thread, so nothing else can be between a lock
 * and its unlock, and both are nothing. They exist so a program that numbers
 * something under a lock means the same on either backend.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

void rho_lock(const char* name)
{
    (void)name;
}

void rho_unlock(const char* name)
{
    (void)name;
}
