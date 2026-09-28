//
// @file main.go
// @brief The rho-sql-sqlite plugin.
//
// The plugin is dispatched to rather than compiled into a service: it is
// reached over the shared plugin protocol, one JSON request per line, and it
// holds its connection for as long as a caller keeps asking.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package main

import (
	"os"

	"github.com/rho/rho-sql/src/rhosql"
)

func main() {
	// The connection is opened here rather than per statement: a database open
	// per statement would make the session pointless, and a service that runs
	// five hundred queries would open its database five hundred times.
	e, err := open()
	if err != nil {
		// A plugin that cannot reach its database says so and answers nothing,
		// rather than accepting statements it will fail to run.
		os.Stderr.WriteString("rho-sql-sqlite: " + err.Error() + "\n")
		os.Exit(1)
	}
	defer e.close()

	rhosql.Serve(os.Stdin, os.Stdout, e)
}
