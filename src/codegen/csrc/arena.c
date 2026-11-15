/*
 * arena.c — where a program's strings live.
 *
 * Every string the runtime builds — a concatenation, a number written as text, a
 * slice, a field read out of a document — is taken from here rather than from
 * `malloc`, and nothing frees one string at a time. The compiler releases them in
 * bulk instead: it takes a mark at the top of a loop and releases back to it at the
 * end of every iteration, and it does the same around a statement whose temporaries
 * nothing keeps. A string a `let` keeps is moved down to the statement's mark, so
 * only the string that is kept remains. A loop that replaces the strings held by
 * variables declared outside it does the same with several at once at the end of
 * each iteration (rho_arena_keep_many): those strings are moved down to the
 * loop's mark and everything else its iterations built is given back.
 *
 * That is sound because a Rho string can only outlive the iteration it was built in
 * by being returned, stored in something declared outside the loop, written through
 * a pointer, or kept by a C function — and the compiler sees the first three: it
 * keeps what a loop stores in a variable it can name, and does not reset a loop
 * that stores a string anywhere else. A C function that keeps a string it
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
 * SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
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

/* The lengths of the last few long arena strings measured (rho_str_len), so
 * that a loop reading one long string a byte or a cut at a time — str_at,
 * str_slice, len — measures it once rather than on every call, which made a
 * scan of a file quadratic on C and linear on Go. Only a string the arena holds
 * is remembered, because the arena is the only writer of its memory: every
 * write goes through rho_str_alloc or a keep's move, and each forgets a length
 * whose bytes it hands out or writes over. A short string is measured every
 * time: that costs little, and remembering it would push out a long one. */
#define RHO_LEN_MEMO_SLOTS 4u
#define RHO_LEN_MEMO_MIN 64u
static struct {
    const char* s;
    size_t n;
} rho_len_memo[RHO_LEN_MEMO_SLOTS];
static unsigned rho_len_memo_next;

/* The bytes [p, p+n) are about to be written: a remembered length they cover
 * is no longer known. */
static void rho_len_memo_written(const char* p, size_t n)
{
    for (unsigned i = 0; i < RHO_LEN_MEMO_SLOTS; i++) {
        const char* s = rho_len_memo[i].s;
        if (s != NULL && (uintptr_t)p <= (uintptr_t)s + rho_len_memo[i].n && (uintptr_t)s < (uintptr_t)p + n) {
            rho_len_memo[i].s = NULL;
        }
    }
}

/* The most strings rho_arena_keep_many is given at once; the compiler keeps no
 * more than this across a loop's iterations (types/arena.go, arenaKeepMax). */
#define RHO_ARENA_KEEP_MAX 32u

/* The strings rho_arena_keep_many keeps, by where they are in the arena: the
 * first `n` entries of `order` index `slots` from the lowest position up. */
static void rho_arena_keep_order(const size_t* position, unsigned char* order, size_t n)
{
    for (size_t i = 1; i < n; i++) {
        unsigned char moving = order[i];
        size_t j = i;
        while (j > 0 && position[order[j - 1]] > position[moving]) {
            order[j] = order[j - 1];
            j--;
        }
        order[j] = moving;
    }
}

/* How far a string kept from `at` reaches: to the end of the furthest string that
 * begins inside it. Two variables may hold one string, or one may hold the tail
 * of another — what str_trim_prefix answers — and the bytes they share are
 * moved once. */
static size_t rho_arena_keep_extent(const size_t* position, char* const* text, const unsigned char* order,
                                    size_t k, size_t i, size_t* next)
{
    size_t end = position[order[i]] + strlen(text[order[i]]) + 1u;
    size_t j = i + 1u;
    while (j < k && position[order[j]] < end) {
        size_t reach = position[order[j]] + strlen(text[order[j]]) + 1u;
        if (reach > end) {
            end = reach;
        }
        j++;
    }
    *next = j;
    return end - position[order[i]];
}

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
    rho_len_memo_written(at, n);
    return at;
}

/* Whether a string lies in what the arena holds now. */
static bool rho_arena_holds(const char* s)
{
    return (uintptr_t)s >= (uintptr_t)rho_arena_buffer && (uintptr_t)s < (uintptr_t)(rho_arena_buffer + rho_arena_top);
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
        rho_len_memo_written(to, n);
        memmove(to, s, n);
        size_t end = mark + n;
        RHO_ARENA_POISON(rho_arena_buffer + end, rho_arena_top - end);
        rho_arena_top = end;
        return to;
    }
    rho_arena_release(mark);
    return s;
}

/* Keeps every string the `n` slots point at that was built since `mark` — a
 * slot is the address of a variable holding a string — and gives back the rest.
 * The strings are moved down to the mark in the order they lie in, so none is
 * written over before it is read, and each variable is pointed at its string's
 * new place. A string from before the mark, a literal, or one C owns is left
 * where it is. It is what ends an iteration of a loop that replaces strings in
 * variables declared outside it. */
void rho_arena_keep_many(size_t mark, void** slots, size_t n)
{
    if (mark >= rho_arena_top) {
        return;
    }
    if (n > RHO_ARENA_KEEP_MAX) {
        panicf("rho: too many strings kept across a loop\n");
        return;
    }
    size_t position[RHO_ARENA_KEEP_MAX];
    char* text[RHO_ARENA_KEEP_MAX];
    unsigned char order[RHO_ARENA_KEEP_MAX];
    size_t k = 0;
    for (size_t i = 0; i < n; i++) {
        char* s = *(char**)slots[i];
        if (s != NULL && s >= rho_arena_buffer + mark && s < rho_arena_buffer + rho_arena_top) {
            position[i] = (size_t)(s - rho_arena_buffer);
            text[i] = s;
            order[k++] = (unsigned char)i;
        }
    }
    rho_arena_keep_order(position, order, k);
    size_t to = mark;
    for (size_t i = 0; i < k;) {
        size_t next = i;
        size_t start = position[order[i]];
        size_t length = rho_arena_keep_extent(position, text, order, k, i, &next);
        char* dest = rho_arena_buffer + to;
        rho_len_memo_written(dest, length);
        memmove(dest, rho_arena_buffer + start, length);
        for (size_t j = i; j < next; j++) {
            *(char**)slots[order[j]] = dest + (position[order[j]] - start);
        }
        to += length;
        i = next;
    }
    if (to < rho_arena_top) {
        RHO_ARENA_POISON(rho_arena_buffer + to, rho_arena_top - to);
    }
    rho_arena_top = to;
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
            rho_len_memo_written(at, n);
            return at;
        }
        if (c != NULL && c->next != NULL) {
            /* A chunk released earlier: empty, and reused from its start. */
            rho_arena_current = c->next;
            rho_arena_current->used = 0;
            continue;
        }
        /* A string longer than a chunk gets a chunk four times its size: an
         * accumulator a loop keeps (`acc = acc + x`) then finds room for its
         * next value beside its current one, and the chunks it needs double
         * rather than grow by one string's length every iteration. */
        size_t size = RHO_STRING_ARENA_CHUNK;
        if (n > RHO_STRING_ARENA_CHUNK) {
            size = n <= ((size_t)-1) / 4u ? 4u * n : n;
        }
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

/* Whether a string lies in what the arena holds now: in the current chunk or
 * one before it, below where it is used up to. */
static bool rho_arena_holds(const char* s)
{
    for (rho_arena_chunk* c = rho_arena_current; c != NULL; c = c->prev) {
        if ((uintptr_t)s >= (uintptr_t)c->data && (uintptr_t)s < (uintptr_t)(c->data + c->used)) {
            return true;
        }
    }
    return false;
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
    rho_len_memo_written(to, n);
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

/* What rho_arena_keep_many does on a board, across chunks: a kept string may be
 * in any chunk from the mark's on, and is moved down to the mark or, when it
 * does not fit in what is left of a chunk, to the start of the next — at the
 * latest its own, which holds it. The strings are taken in the order of their
 * positions, so every move is downwards and reads nothing already written. */
void rho_arena_keep_many(size_t mark, void** slots, size_t n)
{
    if (mark >= rho_arena_position()) {
        return;
    }
    if (n > RHO_ARENA_KEEP_MAX) {
        panicf("rho: too many strings kept across a loop\n");
        return;
    }
    rho_arena_chunk* at = rho_arena_chunk_at(mark);
    if (at == NULL) {
        return;
    }
    rho_arena_chunk* last = rho_arena_current;
    size_t offset = mark - at->base;
    size_t position[RHO_ARENA_KEEP_MAX];
    char* text[RHO_ARENA_KEEP_MAX];
    rho_arena_chunk* home[RHO_ARENA_KEEP_MAX];
    unsigned char order[RHO_ARENA_KEEP_MAX];
    size_t k = 0;
    for (size_t i = 0; i < n; i++) {
        char* s = *(char**)slots[i];
        if (s == NULL) {
            continue;
        }
        for (rho_arena_chunk* c = at; c != NULL; c = c->next) {
            size_t from = c == at ? offset : 0;
            if (s >= c->data + from && s < c->data + c->used) {
                position[i] = c->base + (size_t)(s - c->data);
                text[i] = s;
                home[i] = c;
                order[k++] = (unsigned char)i;
                break;
            }
            if (c == last) {
                break;
            }
        }
    }
    rho_arena_keep_order(position, order, k);
    rho_arena_chunk* into = at;
    size_t to = offset;
    for (size_t i = 0; i < k;) {
        size_t next = i;
        size_t start = position[order[i]];
        size_t length = rho_arena_keep_extent(position, text, order, k, i, &next);
        rho_arena_chunk* from = home[order[i]];
        while (into != from && into->size - to < length) {
            /* What is left of this chunk is too small: nothing kept is in it any
             * more, and the next string starts the next chunk. */
            if (into->used > to) {
                RHO_ARENA_POISON(into->data + to, into->used - to);
            }
            into->used = to;
            into = into->next;
            to = 0;
        }
        char* dest = into->data + to;
        char* source = from->data + (start - from->base);
        RHO_ARENA_UNPOISON(dest, length);
        rho_len_memo_written(dest, length);
        memmove(dest, source, length);
        for (size_t j = i; j < next; j++) {
            *(char**)slots[order[j]] = dest + (position[order[j]] - start);
        }
        to += length;
        if (into->used < to) {
            into->used = to;
        }
        i = next;
    }
    /* Only now is anything given back: every copy read from what is released. */
    if (into->used > to) {
        RHO_ARENA_POISON(into->data + to, into->used - to);
    }
    into->used = to;
    if (into != last) {
        for (rho_arena_chunk* c = into->next; c != NULL; c = c->next) {
            if (c->used != 0) {
                RHO_ARENA_POISON(c->data, c->used);
                c->used = 0;
            }
            if (c == last) {
                break;
            }
        }
    }
    rho_arena_current = into;
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

/* The length of a string, remembered for the last string the arena holds that
 * was measured: a loop that reads one string by position measures it once. A
 * literal or a string C owns is measured every time, since nothing here sees
 * what writes it. */
size_t rho_str_len(const char* s)
{
    if (s == NULL) {
        return 0;
    }
    for (unsigned i = 0; i < RHO_LEN_MEMO_SLOTS; i++) {
        if (rho_len_memo[i].s == s) {
            return rho_len_memo[i].n;
        }
    }
    size_t n = strlen(s);
    if (n >= RHO_LEN_MEMO_MIN && rho_arena_holds(s)) {
        unsigned slot = rho_len_memo_next;
        rho_len_memo_next = (slot + 1u) % RHO_LEN_MEMO_SLOTS;
        rho_len_memo[slot].s = s;
        rho_len_memo[slot].n = n;
    }
    return n;
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

/* A closure's environment (types/closures.go): the values it captured, kept in
 * the arena so it lives exactly as long as a string built in the same place.
 * Aligned for any value, which a string's bytes need not be. */
void* rho_env_alloc(size_t n)
{
    const size_t align = 16u;
    char* raw = rho_str_alloc(n + align - 1u);
    if (raw == NULL) {
        return NULL;
    }
    return (void*)(((uintptr_t)raw + (align - 1u)) & ~(uintptr_t)(align - 1u));
}
