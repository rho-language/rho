/*
 * exec.c — running a program.
 *
 * The runtime's way of running another program and finding out what it said.
 * A tool that orchestrates other tools has to run them, and a program that
 * cannot is a program that can only describe.
 *
 * The same answers as the Go runtime gives, because a program that means one
 * thing on a host and another elsewhere is a program nobody can reason about.
 * The arguments are split on whitespace and honouring quotes, so a name with a
 * space in it is one argument and not two; the program is run directly rather
 * than through a shell, so nothing in an argument can become a command.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include <sys/types.h>
#include <sys/wait.h>
#include <unistd.h>
#include <fcntl.h>

/* The arguments of a command, split on whitespace and honouring quotes. */
static char** rho_split_args(const char* arguments, size_t* count)
{
    size_t capacity = 8;
    size_t length = 0;
    char** args = (char**)malloc(capacity * sizeof(char*));
    if (args == NULL) {
        *count = 0;
        return NULL;
    }
    args[0] = NULL;

    char* current = (char*)malloc(strlen(arguments) + 1);
    if (current == NULL) {
        free(args);
        *count = 0;
        return NULL;
    }
    size_t current_length = 0;
    int quoted = 0;
    int started = 0;

    for (size_t index = 0; arguments[index] != '\0'; index++) {
        char character = arguments[index];
        if (character == '"') {
            quoted = !quoted;
            started = 1;
            continue;
        }
        if (character == ' ' && !quoted) {
            if (current_length > 0 || started) {
                current[current_length] = '\0';
                if (length + 2 > capacity) {
                    capacity *= 2;
                    char** grown = (char**)realloc(args, capacity * sizeof(char*));
                    if (grown == NULL) {
                        break;
                    }
                    args = grown;
                }
                args[length++] = current;
                current = (char*)malloc(strlen(arguments) + 1);
                if (current == NULL) {
                    break;
                }
                current_length = 0;
                started = 0;
            }
            continue;
        }
        current[current_length++] = character;
        started = 1;
    }
    if (current_length > 0 || started) {
        current[current_length] = '\0';
        if (length + 2 > capacity) {
            capacity *= 2;
            char** grown = (char**)realloc(args, capacity * sizeof(char*));
            if (grown == NULL) {
                free(current);
                free(args);
                *count = 0;
                return NULL;
            }
            args = grown;
        }
        args[length++] = current;
    } else {
        free(current);
    }
    args[length] = NULL;
    *count = length;
    return args;
}

/* rho_exec runs a program and writes what it said to a file.
 *
 * The output goes to a file rather than back to the caller, because what a
 * program prints is not a string a caller can be trusted to have room for, and
 * a file is what a reader can read in pieces. The report's place is made
 * first: a run that cannot write its own report is a run whose findings nobody
 * can read.
 *
 * The answer is the program's own exit code, or 255 when the program could not
 * be run at all — which is not a status, and is the one answer that is not one.
 */
unsigned int rho_exec(const char* program, const char* arguments, const char* output_path)
{
    size_t count = 0;
    char** args = rho_split_args(arguments == NULL ? "" : arguments, &count);
    if (args == NULL) {
        return 255;
    }

    /* The program is the first argument, whatever the arguments say. */
    char** argv = (char**)malloc((count + 2) * sizeof(char*));
    if (argv == NULL) {
        free(args);
        return 255;
    }
    argv[0] = (char*)program;
    for (size_t index = 0; index < count; index++) {
        argv[index + 1] = args[index];
    }
    argv[count + 1] = NULL;

    pid_t child = fork();
    if (child < 0) {
        free(argv);
        free(args);
        return 255;
    }
    if (child == 0) {
        /* The child: what it says goes to the report. */
        int output = open(output_path, O_WRONLY | O_CREAT | O_TRUNC, 0644);
        if (output >= 0) {
            dup2(output, STDOUT_FILENO);
            dup2(output, STDERR_FILENO);
            close(output);
        }
        execvp(program, argv);
        /* It could not be run. The parent is told by the exit code. */
        _exit(127);
    }

    int status = 0;
    waitpid(child, &status, 0);
    free(argv);
    free(args);

    if (WIFEXITED(status)) {
        int code = WEXITSTATUS(status);
        return code == 127 ? 255 : (unsigned int)code;
    }
    return 255;
}
