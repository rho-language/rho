# SQL and databases

A service declares its tables and its statements. An engine runs them. Nothing
in between knows what either side is.

(sql:declaring)=
## Declaring

A project says it has a database in one place, in its own manifest:

```yaml
# module.yml
requires:
  - name: rho/sqlite
    version: ^1.0.0

sql:
  driver: rho-sql-sqlite     # the engine that types and runs the statements
  schema: db/schema.yml      # the tables
  out: src/db                # where the generated Rho is written
```

`requires` names the runtime package the built program links, and `driver` names
the engine that answers a statement. They are deliberately different names: the
first is a library, the second is the program that holds it.

(sql:tables)=
### Tables as declarations

A project's tables are declared, not written as SQL. Declaring a table is
declaring a name, its columns and what each holds; the create statement and the
operations over it follow from that:

```yaml
# db/schema.yml
tables:
  - name: customers
    description: The people who buy things.
    columns:
      - name: id
        type: integer
        primary_key: true
        auto: true
      - name: name
        type: text
        not_null: true
        unique: true
      - name: address
        type: text
      - name: balance
        type: number
        default: "0"
```

The list is a list rather than a mapping because declaration order is the order
of the generated struct's fields, and a mapping has none.

What may be declared on a column:

| Key | What it is |
|-----|------------|
| `name` | the column, as the database will call it |
| `type` | what it holds: `integer`, `text`, `float`, `boolean`, `bytes`, `time`, `json`, `number` |
| `description` | the prose a reader of the generated code is looking for |
| `primary_key` | the column is part of the key |
| `auto` | the database generates the value, which only a key may be |
| `not_null` | the column refuses a null |
| `unique` | the column's values are unique |
| `default` | a literal, or an expression such as `datetime('now')` |
| `references` | the table and column a foreign key points at, as `users(id)` |

The vocabulary is a meaning rather than a storage class. `number` is a number the
database keeps exactly, `text` is text of any length, and what a dialect calls
them is the engine's business — a project that moves from one engine to another
changes its driver and not its tables.

A declaration is checked here rather than discovered at run time: a column with
no type, a generated column that is not a key, a table named twice and a type
outside the vocabulary are all reported, with the line, before anything is
generated.

(sql:generated)=
### What is generated

Three things, all from that one file:

- the **create statements**, written as the schema the engine is handed, so the
  database a service creates and the code generated from the same reading cannot
  come to describe two different things;
- the **row types**, one struct per table, which a caller gets as a whole table
  rather than as a shape only one statement happens to produce;
- the **common operations** per table, as named statements the rest of the
  pipeline reads like any other:

| Operation | What it does |
|-----------|--------------|
| `insert_<table>` | adds a row, and answers how many |
| `get_<table>` | the row with this key, or no rows |
| `list_<table>` | every row |
| `update_<table>` | changes the row with this key |
| `delete_<table>` | removes the row with this key |

So a project that declared one table has five typed functions and a create
statement, and wrote neither. What is *not* generated is a statement that is
about the service's domain rather than about its tables — a search with a
visibility rule, a join, a count by owner. Those go in a `queries` file, named,
and are generated the same way:

```sql
-- name: find_customer
-- Returns the customer at this address.
SELECT id, name FROM customers WHERE address = :address;
```

```yaml
# module.yml
sql:
  driver: rho-sql-sqlite
  schema: db/schema.yml
  queries: db/queries.sql
  out: src/db
```

A schema may also be written as SQL — the file's extension says which, and a
project that wrote create statements gets those read as written.

A project with no manifest still works. Its SQL is looked for under `sql/`, and
what is generated is the two standalone backends described below rather than
Rho, because a directory of SQL is not a Rho module.

Two files, then:

```
sql/
  schema.sql     the tables
  queries.sql    the named statements
```

A table is a name, a list of columns, and whatever the service wants to say
about them:

```sql
-- Customers of the catalogue.
CREATE TABLE customers (
    id      integer PRIMARY KEY,          -- the customer's identity
    address text NOT NULL,
    name    text NOT NULL,
    limit   integer NOT NULL DEFAULT 20,
    balance numeric(10, 2) NOT NULL
);
```

A statement is a comment naming it, a description, and the statement:

```sql
-- name: find_customer
-- Returns the customer at this address.
SELECT id, name FROM customers WHERE address = :address;
```

The name is what a handler calls and what a generated function is called. The
description above it is the documentation of both, and reaches the generated
file — which is the point: a function's doc comment is the writer's words rather
than a generator's.

```sh
rho sql read .        # what the SQL declares, read
rho sql queries .     # the statements, and what each takes
rho sql ast .         # the contract an engine is handed
```

(sql:parameters)=
## Parameters

The parameters come from the markers, so the file does not declare them twice:

| Marker | What it is |
|--------|------------|
| `:name` | a named parameter |
| `?` | a positional one |
| `$1` | a numbered one |

A marker is not a name — `?` and `$1` are how a statement marks a value — so a
statement written with positional parameters is still typed, and the generated
function names them by their position.

(sql:engine)=
## The engine

An engine is a plugin. It answers four hooks and is reached over the same
protocol every plugin speaks:

| Hook | Asked |
|------|-------|
| `describe` | what this engine is, and what it can be asked |
| `migrate` | create the tables a schema declares |
| `query` | run one statement |
| `describe_query` | what a statement returns, without running it |

```sh
rho sql engines        # the engines that are installed
rho sql engine <name>  # what an engine says about itself
rho sql run . <statement> [value ...]
```

An engine is a **session**: the connection is opened once and held, so five
hundred queries are one database open and a transaction can span statements.

`describe_query` is the hook that makes typed results possible. What a statement
returns is a fact about the database rather than about the text of the query, so
an engine reports it and a generator asks rather than guessing from a select
list. A column's type comes back in the engine's own spelling — `INTEGER` in
sqlite, `bigint` in duckdb, `NUMERIC(10, 2)` for a scaled number — and this
component reduces those to what a value *is* rather than to a name.

A scaled number is carried as **text**, deliberately. `numeric(10, 2)` keeps
two decimal places exactly and a `float64` does not; text is the one type that
keeps the digits, and a caller that needs a number converts it knowingly. The
same reasoning runs the other way: a value crossing the boundary as a float has
already lost a digit, and a large integer is carried as one from end to end.

(sql:generating)=
## Generating

The same reading that an engine is handed is what generated code is written
from:

```sh
rho build .                            # generating happens as part of the build
rho sql generate . -engine rho-sql-sqlite  # or on its own, with the engine typing
```

A Rho module gets Rho: one file per statement, the row types they return, and a
contract the compiler reads:

```
src/db/
  models.rho        the row types
  queries.rho       one function per statement
  statements.json   what the generated functions are, for the port
```

A project that is not a Rho module — a directory of SQL with no manifest — gets
the two standalone backends instead, each to a directory of its own, because a Go
package and a C translation unit cannot share one:

```
queries/
  go/  models.go, queries.go
  c/   models.h,   queries.c
```

**Go** is written against `database/sql`, which every driver sits behind.

**C** is written against a small handle the generated file owns — an opaque
`rho_db`, a `rho_value`, and eight functions. C has no standard every driver sits
behind, so the seam is declared rather than inherited; a caller that links the
sqlite engine provides the functions, and a caller that links another provides
its own.

What is *not* generated is the connection, the pooling or the engine. Those are a
host's business, and a generated file that opened a database would be a service
that could not be moved to another engine — which is the whole reason the engine
is a plugin.

(sql:guessing)=
## What was guessed

Without an engine, a result is typed from the schema. That is honest for a table's
own shape and a guess for a computed one, and a guess that says so:

```go
// These results were taken from the schema rather than from an engine,
// because none was asked: find_customer, orders_for. Run them against an engine
// to have it type them, which it can do better than a schema can.
```

A type the mapping does not place is carried as text and named in a note on the
function, rather than coerced into a neighbour. A column carried as a string
compiles, runs, and returns the value the database sent; a number carried as
zeroes would do none of those things.

(sql:port)=
## Using the generated code

A handler calls a statement by its name. It never writes SQL and never names an
engine: which database the statements run against is a deployment decision, and a
detail the handler knew would be a handler that stopped working when it changed.

The generated files are the project's own module, so a handler includes them the
way it includes any of its own sources:

```rho
module billing

include "db/models.rho"
include "db/queries.rho"

import std.sql
import std.core.slice
import std.core.ptr

fn main() void {
    let opened = sql_open("billing.db")
    if !opened.is_ok {
        return
    }
    let db: SqlDb = opened.ok
    sql_apply(db, "")

    let rows: Slice[Customer] = Slice[Customer]{ Ptr[Customer](0), 0 }
    let found = FindCustomer("1 Bridge Street", db, rows, 16)
    if found.is_ok {
        print("found them")
    }
    sql_close(db)
}
```

What each surface is:

| Call | What it does |
|------|--------------|
| `sql_open(path)` | opens a database and answers a handle, or why it could not |
| `sql_apply(db, "")` | creates the tables the schema declares, and is what a service runs before it takes a request rather than on every one |
| `sql_close(db)` | releases the handle |
| `sql_exec(db, name, sql, values, affected)` | runs a statement that changes rows and answers how many |
| `sql_prepare`/`sql_run`/`sql_read_column`/`sql_finalize` | walk a result a row at a time, without holding every row at once |
| `sql_bind` | binds a value to a prepared statement, by position |
| `sql_last_message(db)` | the engine's own account of the last failure |
| the generated functions | one per statement, typed, returning `Result[u32, SqlError]` |

The declarations are in `std/sql.rho`, and what supplies them is the `sql` port,
which the compiler generates into the project being built. A project that
declares `sql:` gets that port without asking for it, because the code generated
from its statements calls into it.

The port is written against `database/sql`, so the driver is the one deployment
names — `RHO_SQL_DRIVER`, defaulting to `sqlite` — and the database is
`RHO_SQL_DATABASE`. A program generated on a workstation and deployed to a
service does not carry the workstation's driver with it.

Nothing allocates on the Rho side. A generated read takes a slice the caller owns
and answers how many rows it wrote into it, which is what lets the same file run
on a device with a fixed heap and in a service with a collector.

(sql:writing-an-engine)=
## Writing an engine

An engine is a driver and four methods:

```go
package main

import (
    "os"
    "github.com/rho/rho-sql/src/rhosql"
)

type engine struct{ db *sql.DB }

func (e *engine) Describe() rhosql.Engine {
    return rhosql.Engine{
        Name:      "sqlite",
        Markers:   []rhosql.Marker{rhosql.MarkerPositional},
        Returning: true,
    }
}

func main() {
    rhosql.Serve(os.Stdin, os.Stdout, &engine{})
}
```

A hook with no method is answered as unsupported, so an engine that cannot run a
write does not have to write a migrate that refuses everything.
`rho-sql-sqlite` is the worked example, and what it does about the dialect is
written down in that component's own documentation.
