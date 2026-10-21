/*
 * yaml.c — reading a document.
 *
 * The native YAML support: a program asks a file for a value by the name of the
 * thing it wants, and the runtime answers. The same answers on every backend,
 * because a program that means one thing on a host and another on a part is a
 * program nobody can reason about.
 *
 * The path is dotted: `board.connectors` is the connectors of the board, and
 * `module.version` is the version of the module. A document is walked by its
 * indentation, which is what a YAML mapping is: a key, and the block below it
 * that belongs to it. A path that leaves the block it is in names nothing, and
 * a program is told so rather than given an empty answer it might use.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

/* What a lookup found: the value, and what it is. */
typedef struct {
    char* value;
    char  kind[16];
} RhoYamlFound;

/* The kinds a value can be. */
#define RHO_YAML_MISSING "missing"
#define RHO_YAML_STRING  "string"
#define RHO_YAML_NUMBER  "number"
#define RHO_YAML_BOOLEAN "boolean"
#define RHO_YAML_LIST    "list"
#define RHO_YAML_MAP     "map"

/* read_document reads a whole file, or nothing when it cannot be read. */
static char* rho_yaml_document(const char* path)
{
    FILE* file = fopen(path, "rb");
    char* text = NULL;
    size_t length = 0;
    size_t capacity = 0;
    if (file == NULL) {
        return NULL;
    }
    for (;;) {
        if (length + 4096 + 1 > capacity) {
            capacity = capacity == 0 ? 8192 : capacity * 2;
            char* grown = realloc(text, capacity);
            if (grown == NULL) {
                free(text);
                fclose(file);
                return NULL;
            }
            text = grown;
        }
        size_t read = fread(text + length, 1, 4096, file);
        length += read;
        if (read < 4096) {
            break;
        }
    }
    fclose(file);
    if (text == NULL) {
        return NULL;
    }
    text[length] = '\0';
    return text;
}

/* line_at is the nth line of a document, and how long it is. */
static char* rho_yaml_line(char* text, size_t number, size_t* length)
{
    size_t start = 0;
    for (size_t index = 0; index < number; index++) {
        char* break_at = strchr(text + start, '\n');
        if (break_at == NULL) {
            *length = 0;
            return NULL;
        }
        start = (size_t)(break_at - text) + 1;
    }
    char* break_at = strchr(text + start, '\n');
    *length = break_at == NULL ? strlen(text + start) : (size_t)(break_at - (text + start));
    return text + start;
}

/* indent_of is how far a line is indented, and where its text starts. */
static size_t rho_yaml_indent(const char* line, size_t length, const char** text)
{
    size_t index = 0;
    while (index < length && (line[index] == ' ' || line[index] == '\t')) {
        index++;
    }
    *text = line + index;
    return index;
}

/* trimmed is a line's text without its trailing space. */
static void rho_yaml_trim(char* text)
{
    size_t length = strlen(text);
    while (length > 0 && (text[length - 1] == ' ' || text[length - 1] == '\t' ||
                          text[length - 1] == '\r' || text[length - 1] == '\n')) {
        text[--length] = '\0';
    }
}

/* rho_yaml_kind_of says what a value is, from the line it is on and the block
 * below it.
 *
 * A value on its own line is a scalar, and which scalar it is is said by what
 * it looks like. A value that is a block is a list when the block's first item
 * is a dash, and a map when it is a key.
 */
static void rho_yaml_kind_of(const char* value, char* document, size_t number,
                             size_t indent, char* kind, size_t kind_size)
{
    if (value != NULL && value[0] != '\0') {
        if (value[0] == '[') {
            snprintf(kind, kind_size, "%s", RHO_YAML_LIST);
            return;
        }
        if (value[0] == '{') {
            snprintf(kind, kind_size, "%s", RHO_YAML_MAP);
            return;
        }
        int number_only = 1;
        for (size_t index = 0; value[index] != '\0'; index++) {
            char character = value[index];
            if ((character < '0' || character > '9') && character != '.' &&
                character != '-' && character != '+') {
                number_only = 0;
                break;
            }
        }
        if (number_only) {
            snprintf(kind, kind_size, "%s", RHO_YAML_NUMBER);
            return;
        }
        if (strcmp(value, "true") == 0 || strcmp(value, "false") == 0) {
            snprintf(kind, kind_size, "%s", RHO_YAML_BOOLEAN);
            return;
        }
        snprintf(kind, kind_size, "%s", RHO_YAML_STRING);
        return;
    }
    size_t next = 0;
    const char* text = NULL;
    for (size_t index = number;; index++) {
        char* line = rho_yaml_line(document, index, &next);
        if (line == NULL) {
            break;
        }
        size_t length = next;
        size_t current = rho_yaml_indent(line, length, &text);
        if (text == NULL || text[0] == '\0' || text[0] == '#') {
            continue;
        }
        if (current <= indent) {
            break;
        }
        if (text[0] == '-' && (text[1] == ' ' || text[1] == '\0')) {
            snprintf(kind, kind_size, "%s", RHO_YAML_LIST);
            return;
        }
        snprintf(kind, kind_size, "%s", RHO_YAML_MAP);
        return;
    }
    snprintf(kind, kind_size, "%s", RHO_YAML_MISSING);
}

/* rho_yaml_lookup finds a value in a document by its dotted path, and says
 * what it is.
 *
 * The document is walked by its indentation, which is what a YAML mapping is: a
 * key, and the block below it that belongs to it. A path that leaves the block
 * it is in names nothing, which is what `missing` says.
 */
static RhoYamlFound rho_yaml_lookup(const char* path, const char* key)
{
    RhoYamlFound found;
    found.value = strdup("");
    snprintf(found.kind, sizeof(found.kind), "%s", RHO_YAML_MISSING);

    char* document = rho_yaml_document(path);
    if (document == NULL) {
        return found;
    }
    char* work = strdup(key);
    char* part = work;
    size_t number = 0;
    long indent = -1;
    RhoYamlFound result;
    result.value = strdup("");
    snprintf(result.kind, sizeof(result.kind), "%s", RHO_YAML_MISSING);

    for (;;) {
        char* separator = strchr(part, '.');
        if (separator != NULL) {
            *separator = '\0';
        }
        size_t part_length = strlen(part);
        int found_key = 0;
        for (;;) {
            size_t next = 0;
            char* line = rho_yaml_line(document, number, &next);
            if (line == NULL) {
                break;
            }
            const char* text = NULL;
            size_t current = rho_yaml_indent(line, next, &text);
            if (text == NULL || text[0] == '\0' || text[0] == '#') {
                number++;
                continue;
            }
            if (indent >= 0 && (long)current <= indent) {
                break;
            }
            if (text[0] == '-' && (text[1] == ' ' || text[1] == '\0')) {
                number++;
                continue;
            }
            if (strncmp(text, part, part_length) != 0 || text[part_length] != ':') {
                number++;
                continue;
            }
            /* The value: what is on the key's line, without the key, and
             * without the space that separates them.
             *
             * The line is what the value is on, and a line is not the rest of
             * the document: a value that ran on would be the whole file. */
            size_t line_length = next > part_length + 1 ? next - part_length - 1 : 0;
            char* value = (char*)malloc(line_length + 1);
            if (value == NULL) {
                break;
            }
            memcpy(value, text + part_length + 1, line_length);
            value[line_length] = '\0';
            char* start_of_value = value;
            while (*start_of_value == ' ' || *start_of_value == '\t') {
                start_of_value++;
            }
            if (start_of_value != value) {
                memmove(value, start_of_value, strlen(start_of_value) + 1);
            }
            rho_yaml_trim(value);
            size_t value_length = strlen(value);
            if (value_length >= 2 && value[0] == '"' && value[value_length - 1] == '"') {
                value[value_length - 1] = '\0';
                memmove(value, value + 1, value_length - 1);
            }
            free(result.value);
            result.value = value;
            rho_yaml_kind_of(value, document, number + 1, current, result.kind, sizeof(result.kind));
            indent = (long)current;
            number++;
            found_key = 1;
            break;
        }
        if (!found_key) {
            break;
        }
        if (separator == NULL) {
            break;
        }
        part = separator + 1;
    }
    free(work);
    free(document);
    return result;
}

/* rho_yaml_value reads one value out of a YAML file, named by its path.
 *
 * The path is dotted: `board.connectors` is the connectors of the board. A
 * program that reads a document asks for the thing it wants by the name of the
 * thing, rather than by the name of a reader it carries.
 */
char* rho_yaml_value(const char* path, const char* key)
{
    RhoYamlFound found = rho_yaml_lookup(path, key);
    return found.value;
}

/* rho_yaml_kind says what a value is: `string`, `number`, `boolean`, `list`,
 * `map`, or `missing`.
 *
 * A program that reads a document needs to know what it read: a manifest that
 * says a list where it should say a map is a manifest the reader cannot use,
 * and a reader that cannot say what it read cannot report that.
 */
char* rho_yaml_kind(const char* path, const char* key)
{
    RhoYamlFound found = rho_yaml_lookup(path, key);
    char* kind = strdup(found.kind);
    free(found.value);
    return kind;
}

/* rho_yaml_keys returns the keys under a key, one per line.
 *
 * A program that reads a document needs to know what is in it: a registry is
 * read by asking for each of the things it holds, and a program that cannot
 * ask what is there cannot read what is.
 */
char* rho_yaml_keys(const char* path, const char* key)
{
    char* document = rho_yaml_document(path);
    if (document == NULL) {
        return strdup("");
    }
    char* work = strdup(key);
    char* part = work;
    size_t number = 0;
    long indent = -1;

    for (;;) {
        char* separator = strchr(part, '.');
        if (separator != NULL) {
            *separator = '\0';
        }
        size_t part_length = strlen(part);
        int found_key = 0;
        for (;;) {
            size_t next = 0;
            char* line = rho_yaml_line(document, number, &next);
            if (line == NULL) {
                break;
            }
            const char* text = NULL;
            size_t current = rho_yaml_indent(line, next, &text);
            if (text == NULL || text[0] == '\0' || text[0] == '#') {
                number++;
                continue;
            }
            if (indent >= 0 && (long)current <= indent) {
                break;
            }
            if (text[0] == '-' && (text[1] == ' ' || text[1] == '\0')) {
                number++;
                continue;
            }
            if (strncmp(text, part, part_length) != 0 || text[part_length] != ':') {
                number++;
                continue;
            }
            indent = (long)current;
            number++;
            found_key = 1;
            break;
        }
        if (!found_key) {
            break;
        }
        if (separator == NULL) {
            break;
        }
        part = separator + 1;
    }

    /* The keys directly below. */
    char* result = strdup("");
    size_t result_length = 0;
    long child = -1;
    for (;;) {
        size_t next = 0;
        char* line = rho_yaml_line(document, number, &next);
        if (line == NULL) {
            break;
        }
        const char* text = NULL;
        size_t current = rho_yaml_indent(line, next, &text);
        if (text == NULL || text[0] == '\0' || text[0] == '#') {
            number++;
            continue;
        }
        if ((long)current <= indent) {
            break;
        }
        if (child < 0) {
            child = (long)current;
        }
        if ((long)current == child && !(text[0] == '-' && (text[1] == ' ' || text[1] == '\0'))) {
            char name[256];
            size_t length = 0;
            while (length < next && text[length] != ':' && length < sizeof(name) - 1) {
                name[length] = text[length];
                length++;
            }
            name[length] = '\0';
            rho_yaml_trim(name);
            if (length > 0) {
                size_t needed = result_length + strlen(name) + 2;
                char* grown = realloc(result, needed);
                if (grown == NULL) {
                    break;
                }
                result = grown;
                if (result_length > 0) {
                    result[result_length++] = '\n';
                }
                memcpy(result + result_length, name, strlen(name) + 1);
                result_length += strlen(name);
            }
        }
        number++;
    }
    free(work);
    free(document);
    return result;
}

/* rho_yaml_load reads the name out of a file, the way the configuration of a
 * program is read.
 *
 * The name is the one under the `user:` section, which is where a program's
 * configuration keeps it. It is read through the same walk as everything else,
 * so that a program which asks for `user.name` and a program which loads a
 * configuration are asking the same question.
 */
char* rho_yaml_load(const char* path)
{
    RhoYamlFound found = rho_yaml_lookup(path, "user.name");
    if (strcmp(found.kind, RHO_YAML_MISSING) == 0) {
        free(found.value);
        return strdup("unknown");
    }
    return found.value;
}
