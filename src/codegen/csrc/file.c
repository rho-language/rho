/*
 * file.c — reading and writing a file.
 *
 * Separate from strings.c because it is the only part of the runtime that
 * touches a file, and a program that never does should not carry it. On a board
 * there are no files, and this returns the same answer the platform's console
 * gives for an open that cannot work: a value, and a failure the program can
 * see.
 *
 * SPDX-License-Identifier: Apache-2.0
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
        return strdup("");
    }
    for (;;) {
        if (length + 4096 + 1 > capacity) {
            capacity = capacity == 0 ? 8192 : capacity * 2;
            char* grown = realloc(result, capacity);
            if (grown == NULL) {
                free(result);
                fclose(file);
                return strdup("");
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
        return strdup("");
    }
    result[length] = '\0';
    return result;
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
