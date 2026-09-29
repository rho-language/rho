# rho-sql

A service's SQL, read once and run by whichever engine is installed.

```sh
rho sql read <project>         # what the SQL declares
rho sql ast <project>          # the same, as the contract engines are given
rho sql queries <project>      # the statements, and what each takes
rho sql engines                # the engines that are installed
rho sql engine <name>         # what an engine says about itself
rho sql run <project> <name>  # run one statement and print what came back
```

## The idea

A `.sql` file is written in one engine's dialect, and the differences between
two engines are small but not zero: how a parameter is marked, whether a write
can return the rows it changed, how a type is spelled. Those differences are
why a service that works against sqlite and the same service against duckdb
would otherwise have to be written twice.

So a service declares what it means, and an engine says how to say it:

```
sql/
  schema.sql     the tables
  queries.sql    the named statements
```

```sql
-- schema.sql

-- Customers of the catalogue.
CREATE TABLE customers (
    id      integer PRIMARY KEY,     -- the customer's identity
    address text NOT NULL,          -- where the order goes
    name    text NOT NULL
);
```

```sql
-- queries.sql

-- name: find_customer
-- Returns the customer at this address.
-- The address is compared as written: a collation is an engine's decision.
SELECT id, name FROM customers WHERE address = :address;

-- name: add_customer
-- Adds a customer, and answers with the one that was added.
INSERT INTO customers (address, name) VALUES (:address, :name) RETURNING id;
```

`rho sql read` prints what that declares, and `rho sql ast` prints the contract
an engine is handed. The parameters come out with the marker they were written
in, because which marker an engine takes is most of what differs between them.

The Rho surface a project's generated statements are written against is
`std.sql`, which the standard library declares and the compiler's `sql` port
supplies. It lives in the standard library rather than in this component's
manifest because every project that declares `sql:` gets it: a project should not
have to depend on the generator to have the library its own generated code
imports. The component publishes no interface of its own, so the surface is
declared once.

## What an engine is

A plugin. It answers four hooks and is reached over the same protocol every
plugin speaks:

| Hook | Asked |
|------|------|
| `describe` | what this engine is, and what it can be asked |
| `migrate` | create the tables a schema declares |
| `query` | run one statement |
| `describe_query` | what a statement returns, without running it |

An engine is a session, not a program per statement: the connection is opened
once and held, so five hundred queries are one database open. A transaction can
span statements, which a process per statement could not do.

`describe_query` is the one that makes typed results possible. The columns a
statement returns are a fact about the database rather than about the text of the
query, so an engine reports them and a caller that wants a typed struct asks the
engine rather than guessing from the select list.

## Writing one

An engine is a driver and four bodies:

```go
package main

import (
    "os"
    "github.com/rho/rho-sql/src/rhosql"
)

type engine struct{ db *sql.DB }

func (e *engine) Describe() rhosql.Engine {
    return rhosql.Engine{
        Name:       "sqlite",
        Markers:    []rhosql.Marker{rhosql.MarkerPositional},
        Returning:  true,
    }
}

func main() {
    rhosql.Serve(os.Stdin, os.Stdout, &engine{})
}
```

A hook the engine has no method for is answered as unsupported, so an engine
that cannot run a write does not have to write a migrate that refuses
everything. `rho-sql-sqlite` is the worked example.

## Why the engine is a plugin

Because a service that names its database cannot be moved. A handler that knows
whether it is talking to sqlite or duckdb is a handler that stops working when
that changes, and the shape of the SQL a service declares is the one thing that
should not move when the engine does.

## Building

```sh
cd modules/components/rho-sql && go build -o rho-sql ./src
```

## What generating touches

`rho sql generate` works against a **scratch database** of its own. Describing a
statement needs it prepared, preparing it needs the tables, and creating them in
the database a service runs against would make a command that writes files into
a command that changes production. `rho sql run` is the other way round: it is a
person trying a statement out, so it uses the configured database and says so.
