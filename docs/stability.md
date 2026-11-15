# Stability

Rho is before 1.0, and it still changes. This page says which changes a
program can be broken by, how much warning it gets first, and where the edit to
make is written down. It is a promise about how change arrives, not that
nothing will change.

## Versions

A release is `MAJOR.MINOR.PATCH`, and until 1.0 the major number is 0.

- **A patch release never breaks a program.** `0.17.1` fixes what `0.17.0` got
  wrong and adds nothing a program has to change for.
- **A minor release may break a program, but only something that was
  deprecated in an earlier minor release.** Before a name, a signature or a
  behaviour that is covered below goes away, a release marks it deprecated, and
  every use of it is a compiler warning naming the replacement. It is removed,
  or changed, in a later minor release, never in the one that deprecated it.
- **The exception is a bug that produced wrong code or was unsound.** When
  something compiled and then did the wrong thing — compiled differently on C
  and on Go, read past an array, or was undefined behaviour in the generated C
  — it is refused at once, in the release that finds it, with the fix written in
  the refusal and in the release notes. Keeping it working for a release would
  mean keeping a wrong program running. Something that never compiled, or
  never linked, on any backend is not a break when it is refused by name.

## What is stable

These are covered by the deprecation rule:

- **The language**: the syntax and the meaning described in the
  {ref}`language reference <language/index>` — {ref}`syntax <language/syntax>`,
  {ref}`types <language/types>` and the {ref}`built-in functions
  <language/builtins>` — including what each operation does on each backend.
  What is listed there as a known limitation is a limit, not a promise that it
  stays.
- **The standard library modules tagged `stable`.** Every module of `std/` says
  in its header whether it is `stable` or `experimental`
  (`// @stability stable`); the {ref}`standard library <language/stdlib>` page
  lists each, and the API reference shows the tag at the top of the module's
  page.
- **The module manifest**: the keys of `module.yml` that the
  {ref}`modules <modules/modules>` pages document, and what each means.
- **The CLI**: the commands and flags of `rho` documented in
  {ref}`the rho command <tools/cli>`, and their exit codes.
- **The diagnostic format**: `file:line:col: severity: message`, with notes on
  the lines after it as `file:line:col: note: message`, which an editor's problem
  matcher reads. The wording of a message is not covered: it is improved
  whenever a message can say more clearly what is wrong.

## What may change

These may change in any minor release, without a deprecation first. A change
to one is still written in the release notes, with the edit when there is one.

- **The standard library modules tagged `experimental`**, and anything else the
  documentation marks experimental.
- **The internals of a component**: its Go packages, the files it writes for
  itself, and a subcommand or flag its own page does not document. A
  component's documented commands follow its own changelog.
- **The layout of the generated C and Go projects**: file names, the names of
  generated symbols and types, and how a construct is lowered. A program should
  read what it built through the documented outputs — the binary, the
  documented ports — not through the generated sources.
- **The interface of a port** — the functions a host implements for `logemit`,
  `sql` or `httproute` — except where a page documents it as the interface to
  implement.
- **Warnings**: a release may warn about something it did not warn about
  before. A warning never stops `rho build`.

## How a change arrives

**A deprecation warning, first.** A declaration whose doc comment carries a
`// @deprecated` line is deprecated, and every use of it from another module is
a warning at the use:

```text
src/main.rho:7:18: warning: read_all is deprecated: use read_bytes instead
lib/files.rho:12:1: note: read_all is declared here
```

The rest of the `@deprecated` line is the replacement, so the warning says what
to write. The tag works on a function, a struct, an enum, an enum variant, a
constant and a type alias; on a whole module it goes in the header above the
`module` line, and each `import` of the module is the warning. The warning is the
same in `rho build`, in `rho lint` and in the editor, which strikes the use
through. {ref}`language/syntax` has the details. The tag is open to every
module, not only the standard library: a module published to a registry can
deprecate its own names the same way.

**A "Breaking" section in the release notes.** Every release note in
`CHANGELOG.md`, from 0.11.0 on, has a `### Breaking` section listing each change
a program may need to be edited for, and the exact edit:

```markdown
### Breaking

- `std.os.sys`: `write(fd, buf, len)` and `read(fd, buf, len)` take
  `buf: Slice[u8]` instead: `write(fd, buf[..n])`.
```

A release that breaks nothing says `None.` there. A component's changelog does
the same for a change to its documented commands.

**Mechanical edits.** There is no `rho fix` yet, and `rho fmt` does not rewrite
code: an edit in a Breaking section is made by hand, which is why each one is
written out in full. A tool that applies the mechanical ones is future work.
`rho fmt` also does not yet keep comments, so formatting a file drops its doc
comments and the `@deprecated` and `@stability` tags in them.

## After 1.0

The same rules, with the major number in place of the minor one: a change that
breaks a program covered above waits for the next major release and is a
deprecation warning before it.
