/*
 * arena.c — where a program's strings live.
 *
 * Every string the runtime builds — a concatenation, a number written as text, a
 * slice, a field read out of a document — is taken from here rather than from
 * `malloc`, and nothing frees one string at a time. The compiler releases them in
 * bulk instead: it takes a mark at the top of a loop and releases back to it at the
 * end of every iteration, and it does the same around a statement whose temporaries
 * nothing keeps. A string a `let` keeps is moved down to the statement's mark, so
 * only the string that is kept remains.
 *
 * That is sound because a Rho string can only outlive the iteration it was built in
 * by being returned, stored in something declared outside the loop, written through
 * a pointer, or kept by a C function — and the compiler sees the first three and
 * does not reset a loop that does any of them. A C function that keeps a string it
 * was given copies it: a string passed to C is borrowed for the duration of the
 * call.
 *
 * Two shapes, chosen at compile time:
 *
 * - On a board, a fixed buffer of RHO_STRING_ARENA_BYTES, set by the target spec
 *   (`linker.string_arena_bytes`). Running out stops the program with
 *   "string arena exhausted" rather than corrupting the memory beside it, and the
 *   program needs no `malloc` for its strings at all.
 * - On a host, a list of chunks that grows when one is full. A chunk is never
 *   given back; a released one is reused by the next iteration.
 *
 * A position in the arena is one number, which is what a mark is: chunks keep the
 * position their first byte has, so a mark taken in one chunk and released from a
 * later one still names the same place.
 *
 * Under AddressSanitizer a released region is poisoned, so a string used after the
 * iteration that built it is reported where it is read rather than read as
 * whatever the next iteration wrote there.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

/* A bare-metal part with no size from its target spec still gets a fixed arena:
 * a growing one would need the very `malloc` this exists to keep out of a
 * firmware. Linux on an ARM board is a host, and grows. */
#if !defined(RHO_STRING_ARENA_BYTES) && !defined(RHO_STRING_ARENA_GROWS)
#if (defined(__arm__) || defined(__thumb__) || defined(__riscv) || defined(__AVR__) || \
     defined(__XTENSA__) || defined(__xtensa__)) && !defined(__linux__) && !defined(__APPLE__)
#if defined(__AVR__)
#define RHO_STRING_ARENA_BYTES 256
#else
#define RHO_STRING_ARENA_BYTES 4096
#endif
#endif
#endif

#if defined(__SANITIZE_ADDRESS__)
#define RHO_ARENA_ASAN 1
#elif defined(__has_feature)
#if __has_feature(address_sanitizer)
#define RHO_ARENA_ASAN 1
#endif
#endif

#if defined(RHO_ARENA_ASAN)
void __asan_poison_memory_region(void const volatile* addr, size_t size);
void __asan_unpoison_memory_region(void const volatile* addr, size_t size);
#define RHO_ARENA_POISON(p, n) __asan_poison_memory_region((p), (n))
#define RHO_ARENA_UNPOISON(p, n) __asan_unpoison_memory_region((p), (n))
#else
#define RHO_ARENA_POISON(p, n) ((void)(p), (void)(n))
#define RHO_ARENA_UNPOISON(p, n) ((void)(p), (void)(n))
#endif

#if defined(RHO_STRING_ARENA_BYTES) && !defined(RHO_STRING_ARENA_GROWS)

/* ---- A board: one fixed buffer. ---------------------------------------- */

static char rho_arena_buffer[RHO_STRING_ARENA_BYTES];
static size_t rho_arena_top;

char* rho_str_alloc(size_t n)
{
    if (n > (size_t)RHO_STRING_ARENA_BYTES - rho_arena_top) {
        panicf("rho: string arena exhausted\n");
        return NULL;
    }
    char* at = rho_arena_buffer + rho_arena_top;
    rho_arena_top += n;
    RHO_ARENA_UNPOISON(at, n);
    return at;
}

size_t rho_arena_mark(void)
{
    return rho_arena_top;
}

void rho_arena_release(size_t mark)
{
    if (mark < rho_arena_top) {
        RHO_ARENA_POISON(rho_arena_buffer + mark, rho_arena_top - mark);
        rho_arena_top = mark;
    }
}

const char* rho_arena_keep(size_t mark, const char* s)
{
    if (s != NULL && mark < rho_arena_top && s >= rho_arena_buffer + mark &&
        s < rho_arena_buffer + rho_arena_top) {
        size_t n = strlen(s) + 1;
        char* to = rho_arena_buffer + mark;
        memmove(to, s, n);
        size_t end = mark + n;
        RHO_ARENA_POISON(rho_arena_buffer + end, rho_arena_top - end);
        rho_arena_top = end;
        return to;
    }
    rho_arena_release(mark);
    return s;
}

size_t rho_arena_used(void)
{
    return rho_arena_top;
}

#else

/* ---- A host: chunks that grow. ----------------------------------------- */

#ifndef RHO_STRING_ARENA_CHUNK
#define RHO_STRING_ARENA_CHUNK 65536u
#endif

typedef struct rho_arena_chunk {
    struct rho_arena_chunk* prev;
    struct rho_arena_chunk* next;
    /* The position of data[0]: the sum of the sizes of every chunk before. */
    size_t base;
    size_t size;
    size_t used;
    char data[];
} rho_arena_chunk;

static rho_arena_chunk* rho_arena_current;

static size_t rho_arena_position(void)
{
    return rho_arena_current != NULL ? rho_arena_current->base + rho_arena_current->used : 0;
}

/* Everything after `chunk` is empty: a later chunk is free space to reuse. */
static void rho_arena_empty_after(rho_arena_chunk* chunk)
{
    for (rho_arena_chunk* c = chunk != NULL ? chunk->next : NULL; c != NULL && c->used != 0; c = c->next) {
        RHO_ARENA_POISON(c->data, c->used);
        c->used = 0;
    }
}

/* The chunk a position falls in, walking back from the current one. */
static rho_arena_chunk* rho_arena_chunk_at(size_t position)
{
    rho_arena_chunk* c = rho_arena_current;
    while (c != NULL && position < c->base) {
        c = c->prev;
    }
    return c;
}

char* rho_str_alloc(size_t n)
{
    for (;;) {
        rho_arena_chunk* c = rho_arena_current;
        if (c != NULL && c->size - c->used >= n) {
            char* at = c->data + c->used;
            c->used += n;
            RHO_ARENA_UNPOISON(at, n);
            return at;
        }
        if (c != NULL && c->next != NULL) {
            /* A chunk released earlier: empty, and reused from its start. */
            rho_arena_current = c->next;
            rho_arena_current->used = 0;
            continue;
        }
        size_t size = n > RHO_STRING_ARENA_CHUNK ? n : RHO_STRING_ARENA_CHUNK;
        rho_arena_chunk* fresh = (rho_arena_chunk*)malloc(sizeof(rho_arena_chunk) + size);
        if (fresh == NULL) {
            panicf("rho: string arena exhausted (out of memory)\n");
            return NULL;
        }
        fresh->prev = c;
        fresh->next = NULL;
        fresh->base = c != NULL ? c->base + c->size : 0;
        fresh->size = size;
        fresh->used = 0;
        RHO_ARENA_POISON(fresh->data, size);
        if (c != NULL) {
            c->next = fresh;
        }
        rho_arena_current = fresh;
    }
}

size_t rho_arena_mark(void)
{
    return rho_arena_position();
}

void rho_arena_release(size_t mark)
{
    if (mark >= rho_arena_position()) {
        return;
    }
    rho_arena_chunk* c = rho_arena_chunk_at(mark);
    if (c == NULL) {
        return;
    }
    size_t used = mark - c->base;
    if (used < c->used) {
        RHO_ARENA_POISON(c->data + used, c->used - used);
        c->used = used;
    }
    rho_arena_empty_after(c);
    rho_arena_current = c;
}

const char* rho_arena_keep(size_t mark, const char* s)
{
    if (s == NULL || mark >= rho_arena_position()) {
        rho_arena_release(mark);
        return s;
    }
    rho_arena_chunk* at = rho_arena_chunk_at(mark);
    if (at == NULL) {
        return s;
    }
    /* Whether the string was built after the mark: it lies in the mark's chunk at
     * or past the mark, or in a later chunk. Anything else — a literal, a string
     * from before the mark, one a C function owns — is not the arena's to move. */
    size_t offset = mark - at->base;
    rho_arena_chunk* home = NULL;
    for (rho_arena_chunk* c = at; c != NULL; c = c->next) {
        size_t from = c == at ? offset : 0;
        if (s >= c->data + from && s < c->data + c->used) {
            home = c;
            break;
        }
        if (c == rho_arena_current) {
            break;
        }
    }
    if (home == NULL) {
        rho_arena_release(mark);
        return s;
    }
    size_t n = strlen(s) + 1;
    /* Where it goes: at the mark when it fits there, else at the start of the
     * first chunk after that is big enough — at the latest its own, which holds
     * it. Either place is below the string, so the copy moves it down. */
    rho_arena_chunk* into = at;
    size_t start = offset;
    if (at->size - offset < n) {
        /* The string is in a later chunk: one that holds it in the mark's chunk
         * would also fit at the mark. */
        into = at->next;
        while (into != home && into->size < n) {
            into = into->next;
        }
        start = 0;
    }
    char* to = into->data + start;
    RHO_ARENA_UNPOISON(to, n);
    memmove(to, s, n);
    /* Only now is anything given back: the copy read from the region released. */
    if (into != at) {
        for (rho_arena_chunk* c = at->next; c != into; c = c->next) {
            RHO_ARENA_POISON(c->data, c->used);
            c->used = 0;
        }
        if (at->used > offset) {
            RHO_ARENA_POISON(at->data + offset, at->used - offset);
            at->used = offset;
        }
    }
    if (into->used > start + n) {
        RHO_ARENA_POISON(into->data + start + n, into->used - start - n);
    }
    into->used = start + n;
    rho_arena_empty_after(into);
    rho_arena_current = into;
    return to;
}

size_t rho_arena_used(void)
{
    return rho_arena_position();
}

#endif

/* ---- What both shapes share. ------------------------------------------- */

/* A condition's temporaries are released once it has been evaluated: the
 * argument is computed before the call, so everything it built is above the
 * mark and nothing reads it after. */
bool rho_arena_release_cond(size_t mark, bool value)
{
    rho_arena_release(mark);
    return value;
}

/* A string of the runtime's own, copied into the arena. */
char* rho_str_dup(const char* s)
{
    if (s == NULL) {
        s = "";
    }
    size_t n = strlen(s) + 1;
    char* out = rho_str_alloc(n);
    if (out == NULL) {
        return (char*)"";
    }
    memcpy(out, s, n);
    return out;
}

/* A string built in a buffer `malloc` gave — a file read whole, a program's output
 * — moved into the arena, and the buffer given back. */
char* rho_str_adopt(char* heap)
{
    if (heap == NULL) {
        return (char*)"";
    }
    char* out = rho_str_dup(heap);
    free(heap);
    return out;
}
