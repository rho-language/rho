/*
 * file.c — reading and writing a file.
 *
 * Separate from strings.c because it is the only part of the runtime that
 * touches a file, and a program that never does should not carry it. On a board
 * there are no files, and this returns the same answer the platform's console
 * gives for an open that cannot work: a value, and a failure the program can
 * see.
 *
 * SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

/* rho_read_text reads a whole file, or "" when it cannot be read.
 *
 * The text of a file, which is what a program reads a document as: a registry
 * or a manifest is read as text and understood by the program that reads it.
 */
char* rho_read_text(const char* path)
{
    FILE* file = fopen(path, "rb");
    char* result = NULL;
    size_t length = 0;
    size_t capacity = 0;
    if (file == NULL) {
        return (char*)"";
    }
    for (;;) {
        if (length + 4096 + 1 > capacity) {
            capacity = capacity == 0 ? 8192 : capacity * 2;
            char* grown = realloc(result, capacity);
            if (grown == NULL) {
                free(result);
                fclose(file);
                return (char*)"";
            }
            result = grown;
        }
        size_t read = fread(result + length, 1, 4096, file);
        length += read;
        if (read < 4096) {
            break;
        }
    }
    fclose(file);
    if (result == NULL) {
        return (char*)"";
    }
    result[length] = '\0';
    /* Read into a buffer of its own, which grows, and handed back as a string of
     * the arena's: the program releases strings in bulk, never one by one. */
    return rho_str_adopt(result);
}

/* rho_write_text writes a whole file, and says whether it did.
 *
 * A file that is written is a file that replaced what was there, so this says
 * whether it was written: a program that writes a registry and does not know
 * whether it did is a program that cannot say what it did.
 */
int rho_write_text(const char* path, const char* text)
{
    FILE* file = fopen(path, "wb");
    if (file == NULL) {
        return 0;
    }
    size_t length = strlen(text);
    size_t written = fwrite(text, 1, length, file);
    fclose(file);
    return written == length ? 1 : 0;
}

/* rho_write_secret writes a whole file that only its owner can read.
 *
 * A token, a key or a credential in a world-readable file is a secret every user
 * of the machine holds. Where the platform has POSIX files the file is created
 * with mode 0600 before anything is written to it; a board has no files and no
 * other users, and writes it the way any file is written.
 */
#if defined(__unix__) || defined(__APPLE__)
#include <fcntl.h>
#include <sys/stat.h>
#include <unistd.h>
int rho_write_secret(const char* path, const char* text)
{
    int fd = open(path, O_WRONLY | O_CREAT | O_TRUNC, 0600);
    if (fd < 0) {
        return 0;
    }
    /* An existing file keeps its old mode through O_CREAT; set it either way. */
    if (fchmod(fd, 0600) != 0) {
        close(fd);
        return 0;
    }
    size_t length = strlen(text);
    size_t done = 0;
    while (done < length) {
        ssize_t n = write(fd, text + done, length - done);
        if (n <= 0) {
            close(fd);
            return 0;
        }
        done += (size_t)n;
    }
    return close(fd) == 0 ? 1 : 0;
}
#else
int rho_write_secret(const char* path, const char* text)
{
    return rho_write_text(path, text);
}
#endif

/* rho_append_text appends to the end of a file, and says whether it did.
 *
 * A feed that accumulates one event per line — the file an SSE stream reads —
 * is written by appending rather than replacing: append keeps every previous
 * event, and only the newest is missing after a crash. It is how a page hears
 * of a change without asking again.
 */
int rho_append_text(const char* path, const char* text)
{
    FILE* file = fopen(path, "ab");
    if (file == NULL) {
        return 0;
    }
    size_t length = strlen(text);
    size_t written = fwrite(text, 1, length, file);
    fclose(file);
    return written == length ? 1 : 0;
}
