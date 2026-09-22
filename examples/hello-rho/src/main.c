/* Rho → C backend: module hello */

#define _POSIX_C_SOURCE 200809L
#include <stdint.h>
#include <stdbool.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stddef.h>

#define RHO_FEATURE_MATH 1
#define RHO_FEATURE_MATH 1

static char* rho_concat(const char* a, const char* b);
static char* rho_yaml_load(const char* path);

static char* rho_string_from_i32(int32_t value);

int main();
const char* prefix();
const char* message();

int main() {
    const char* name = rho_yaml_load("config/app.yml");
    printf("%s\n", rho_concat(prefix(), name));
}

const char* prefix() {
    return "Hello ";
}

const char* message() {
    return "Rho";
}

static char* rho_concat(const char* a, const char* b) {
    size_t len_a = strlen(a);
    size_t len_b = strlen(b);
    char* out = malloc(len_a + len_b + 1);
    if (!out) return NULL;
    memcpy(out, a, len_a);
    memcpy(out + len_a, b, len_b + 1);
    return out;
}

static char* rho_yaml_load(const char* path) {
    FILE* f = fopen(path, "r");
    char line[4096];
    char* result = NULL;
    int in_user = 0;
    if (!f) return strdup("unknown");
    while (fgets(line, sizeof(line), f) != NULL) {
        char* trimmed = line;
        while (*trimmed == ' ' || *trimmed == '\t' || *trimmed == '\n' || *trimmed == '\r') trimmed++;
        if (*trimmed == '\0' || *trimmed == '#') continue;
        if (strncmp(trimmed, "user:", 5) == 0) { in_user = 1; continue; }
        if (in_user && strncmp(trimmed, "name:", 5) == 0) {
            char* value = trimmed + 5;
            while (*value == ' ' || *value == '\t') value++;
            size_t len = strlen(value);
            while (len > 0 && (value[len - 1] == '\n' || value[len - 1] == '\r')) { value[--len] = '\0'; }
            result = strdup(value);
            break;
        }
    }
    fclose(f);
    if (result != NULL) return result;
    return strdup("unknown");
}

static char* rho_string_from_i32(int32_t value) {
    char buffer[32];
    snprintf(buffer, sizeof(buffer), "%d", value);
    return strdup(buffer);
}

