# SQL and databases

A service declares its tables and its statements. An engine runs them. Nothing
in between knows what either side is.

(sql:declaring)=
## Declaring

A project keeps its SQL in two files, in the project root or in `sql/`:

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
rho sql generate .                     # the schema, the schema's own types
rho sql generate . -engine rho-sqlite  # with the engine typing the results
```

It writes two backends, each to a directory of its own, because a Go package and
a C translation unit cannot share one:

```
queries/
  go/  models.go, queries.go
  c/   models.h,   queries.c
```

**Go** is written against `database/sql`, which every driver sits behind. A
caller opens the handle with the driver it configured:

```go
queries.UseDB(handle)
rows, err := queries.FindCustomer(ctx, "1 Bridge Street")
```

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
