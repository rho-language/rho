/*
 * yaml.c — reading one name out of a YAML file.
 *
 * Separate from strings.c because it is the only part of the runtime that opens a
 * file, and a program that never reads one should not carry it. On a board there
 * are no files, and this returns the same answer the platform's console gives for
 * an open that cannot work: a value, and a failure the program can see.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

char* rho_yaml_load(const char* path)
{
    FILE* f = fopen(path, "r");
    char line[4096];
    char* result = NULL;
    int in_user = 0;
    if (f == NULL) {
        return strdup("unknown");
    }
    while (fgets(line, sizeof(line), f) != NULL) {
        char* trimmed = line;
        while (*trimmed == ' ' || *trimmed == '\t' || *trimmed == '\n' || *trimmed == '\r') {
            trimmed++;
        }
        if (*trimmed == '\0' || *trimmed == '#') {
            continue;
        }
        if (strncmp(trimmed, "user:", 5) == 0) {
            in_user = 1;
            continue;
        }
        if (in_user && strncmp(trimmed, "name:", 5) == 0) {
            char* value = trimmed + 5;
            while (*value == ' ' || *value == '\t') {
                value++;
            }
            size_t len = strlen(value);
            while (len > 0 && (value[len - 1] == '\n' || value[len - 1] == '\r')) {
                value[--len] = '\0';
            }
            result = strdup(value);
            break;
        }
    }
    fclose(f);
    if (result != NULL) {
        return result;
    }
    return strdup("unknown");
}