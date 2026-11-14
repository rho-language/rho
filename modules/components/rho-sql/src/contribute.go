//
// @file contribute.go
// @brief The tables and statements a project's dependencies bring with them.
//
// A module that keeps records — an inventory, a scheduler — owns its tables and
// the statements over them, and says so in its own manifest:
//
//	sql:
//	  schema: db/schema.sql
//	  queries: db/queries.sql
//
// with no driver and no `out`, because a module generates nothing by itself: it
// is compiled into the application that depends on it, and the statements are
// generated into that application. So when an application is generated, every
// module it has locked that declares `sql:` contributes its schema and its
// statements first, and the application's own files follow.
//
// A module's definition is the one used. An application written before modules
// carried their SQL repeats a module's tables and statements in its own files;
// its copy of a table is passed over and its copy of a statement is skipped, with
// one line saying which, so the copies can be removed and the module can change
// its tables without every application that uses it changing too.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rho/rho-sql/src/rhosql"
	"rho/src/deps"
)

// Contribution is one dependency's schema and statements.
type Contribution struct {
	// Module is the dependency's name, as the lock records it.
	Module string
	// Schema and Queries are the files, absolute; either may be empty.
	Schema  string
	Queries string
}

// LockFile is where a project's resolved dependencies are recorded.
const LockFile = "rho.lock"

// Contributions are the locked dependencies of a project that declare `sql:`
// without a driver, in name order so a generated file does not depend on the
// order a resolver happened to write. A project with no lock has none.
func Contributions(project string) []Contribution {
	lock, err := deps.LoadLockfile(filepath.Join(project, LockFile))
	if err != nil || lock == nil {
		return nil
	}
	own, _ := filepath.Abs(project)
	out := []Contribution{}
	for _, pkg := range lock.Packages {
		if pkg.Path == "" {
			continue
		}
		root, err := filepath.Abs(pkg.Path)
		if err != nil || root == own {
			continue
		}
		declaration := LoadDeclaration(root)
		if !declaration.SQL.Declared() || declaration.SQL.Driver != "" {
			// A dependency with a driver is a project of its own that
			// generates its own code; one without `sql:` keeps no records.
			continue
		}
		name := declaration.Name
		if name == "" {
			name = pkg.Name
		}
		out = append(out, Contribution{
			Module:  name,
			Schema:  within(root, declaration.SQL.Schema),
			Queries: within(root, declaration.SQL.Queries),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Module < out[j].Module })
	return out
}

// mergeContributions puts the dependencies' tables and statements before a
// project's own. A table or statement a project repeats is the module's: the
// project's copy is dropped, and a line on stderr names what was dropped.
func mergeContributions(own rhosql.Schema, ownDDL string, contributions []Contribution, warn func(string)) (rhosql.Schema, string, []rhosql.Diagnostic) {
	if len(contributions) == 0 {
		return own, ownDDL, nil
	}
	merged := rhosql.Schema{Version: own.Version, Module: own.Module, Description: own.Description,
		Tables: []rhosql.Table{}, Queries: []rhosql.Query{}}
	tableFrom := map[string]string{}
	queryFrom := map[string]string{}
	querySQL := map[string]string{}
	var diagnostics []rhosql.Diagnostic
	var ddl strings.Builder
	for _, c := range contributions {
		if text := readIfPresent(c.Schema); text != "" {
			parsed, found := rhosql.ParseSchema(c.Module+"/"+filepath.Base(c.Schema), text)
			diagnostics = append(diagnostics, found...)
			for _, table := range parsed.Tables {
				if other, seen := tableFrom[table.Name]; seen {
					// Two modules owning one table would write each other's rows;
					// the application is told which two, rather than one winning.
					diagnostics = append(diagnostics, rhosql.Diagnostic{Level: "error",
						File:    c.Module + "/" + filepath.Base(c.Schema),
						Message: fmt.Sprintf("table %s is declared by both %s and %s", table.Name, other, c.Module)})
					continue
				}
				tableFrom[table.Name] = c.Module
				merged.Tables = append(merged.Tables, table)
			}
			fmt.Fprintf(&ddl, "-- The %s module's tables.\n%s\n", c.Module, strings.TrimRight(text, "\n"))
			ddl.WriteString("\n\n")
		}
		if text := readIfPresent(c.Queries); text != "" {
			parsed, found := rhosql.ParseQueries(c.Module+"/"+filepath.Base(c.Queries), text)
			diagnostics = append(diagnostics, found...)
			for _, query := range parsed.Queries {
				if other, seen := queryFrom[query.Name]; seen {
					diagnostics = append(diagnostics, rhosql.Diagnostic{Level: "error",
						File:    c.Module + "/" + filepath.Base(c.Queries),
						Message: fmt.Sprintf("%s is declared by both %s and %s", query.Name, other, c.Module)})
					continue
				}
				queryFrom[query.Name] = c.Module
				querySQL[query.Name] = query.SQL
				merged.Queries = append(merged.Queries, query)
			}
		}
	}
	repeated := map[string][]string{}
	for _, table := range own.Tables {
		if module, seen := tableFrom[table.Name]; seen {
			repeated[module] = append(repeated[module], "table "+table.Name)
			continue
		}
		merged.Tables = append(merged.Tables, table)
	}
	for _, query := range own.Queries {
		if module, seen := queryFrom[query.Name]; seen {
			// A copy of the module's statement — even an older one that
			// differs — is passed over for the module's; one that differs is
			// said apart, because the project may have meant its own.
			if strings.Join(strings.Fields(query.SQL), " ") != strings.Join(strings.Fields(querySQL[query.Name]), " ") {
				repeated[module] = append(repeated[module], query.Name+" (the project's differs: rename it if it is its own)")
				continue
			}
			repeated[module] = append(repeated[module], query.Name)
			continue
		}
		merged.Queries = append(merged.Queries, query)
	}
	modules := make([]string, 0, len(repeated))
	for module := range repeated {
		modules = append(modules, module)
	}
	sort.Strings(modules)
	for _, module := range modules {
		names := repeated[module]
		warn(fmt.Sprintf("the project repeats %d of %s's definitions (%s); %s's are used, and the copies can be removed",
			len(names), module, strings.Join(names, ", "), module))
	}
	ddl.WriteString(ownDDL)
	return merged, ddl.String(), diagnostics
}

// warnStderr is how a merge reports what it dropped.
func warnStderr(message string) {
	fmt.Fprintf(os.Stderr, "rho sql: %s\n", message)
}
