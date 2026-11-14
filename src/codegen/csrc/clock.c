/*
 * clock.c — the time, read from the process.
 *
 * The same answers as the Go runtime gives: Unix seconds, the local date and
 * time of day, and the local zone's offset from UTC at a moment (a time written
 * last winter is shown with last winter's offset, across a change to summer
 * time). A program that asked `date` and read its answer from a file could read
 * a file another caller was still writing.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

#include <stdlib.h>
#include <string.h>
#include <time.h>

unsigned int rho_now_s(void)
{
    return (unsigned int)time(NULL);
}

static char* rho_clock_format(const char* format)
{
    char buffer[32];
    time_t now = time(NULL);
    struct tm local;
    buffer[0] = '\0';
    if (localtime_r(&now, &local) != NULL) {
        strftime(buffer, sizeof buffer, format, &local);
    }
    return rho_str_dup(buffer);
}

char* rho_local_date(void)
{
    return rho_clock_format("%Y-%m-%d");
}

char* rho_local_clock(void)
{
    return rho_clock_format("%H:%M");
}

/* The offset is the local time less UTC at the same moment, from the two broken
 * down times, since tm_gmtoff is not standard C. They are at most a day apart. */
long long rho_utc_offset_at(long long epoch)
{
    time_t at = (time_t)epoch;
    struct tm local;
    struct tm utc;
    if (localtime_r(&at, &local) == NULL || gmtime_r(&at, &utc) == NULL) {
        return 0;
    }
    long long days = 0;
    if (local.tm_year != utc.tm_year) {
        days = local.tm_year > utc.tm_year ? 1 : -1;
    } else {
        days = (long long)(local.tm_yday - utc.tm_yday);
    }
    return days * 86400LL + (long long)(local.tm_hour - utc.tm_hour) * 3600LL +
        (long long)(local.tm_min - utc.tm_min) * 60LL + (long long)(local.tm_sec - utc.tm_sec);
}
