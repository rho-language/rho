/*
 * os.c — the process and its file descriptors, through the C library.
 *
 * What `std.os.sys`, `std.os.fs` and `std.os.time` call. They used to issue raw
 * system calls by number, which named x86-64's numbers and so meant something
 * else on every other architecture; the C library's wrappers mean the same
 * thing on each.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

#include <fcntl.h>
#include <stdlib.h>
#include <time.h>
#include <unistd.h>

void rho_exit(uint32_t code)
{
    exit((int)code);
}

/* Nanoseconds since the Unix epoch, from the realtime clock: one reading, so
 * the seconds and the fraction a caller splits it into belong together. */
uint64_t rho_time_now_ns(void)
{
    struct timespec now;
    if (clock_gettime(CLOCK_REALTIME, &now) != 0) {
        return 0;
    }
    return (uint64_t)now.tv_sec * 1000000000ULL + (uint64_t)now.tv_nsec;
}

/* The descriptor, or -1. A file created by it can be read and written by its
 * owner and read by everyone else, as one written by `rho_write_text` can. */
int32_t rho_fd_open(const uint8_t* path, uint32_t flags)
{
    return (int32_t)open((const char*)path, (int)flags, 0644);
}

uint32_t rho_fd_close(uint32_t fd)
{
    return (uint32_t)close((int)fd);
}

/* The number of bytes moved, or 0xFFFFFFFF when the call failed. */
uint32_t rho_fd_write(uint32_t fd, const uint8_t* buf, uint32_t len)
{
    return (uint32_t)write((int)fd, buf, (size_t)len);
}

uint32_t rho_fd_read(uint32_t fd, uint8_t* buf, uint32_t len)
{
    return (uint32_t)read((int)fd, buf, (size_t)len);
}
