//
// @file pkg.go
// @brief Rho Package Manager - Enterprise.
//
// Implements `rho pkg`, a minimal package manager for Rho 0.1.
// Supports:
//   - rho pkg init
//   - rho pkg add <module> <url>
//   - rho pkg list
//
// Uses rho.yaml as project manifest.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package main

import (
    "errors"
    "flag"
    "fmt"
    "os"

    "rho/src/yaml"
)

/* -------------------------------------------------------------------------- */
/*  Types                                                                     */
/* -------------------------------------------------------------------------- */

type Manifest struct {
    Name    string
    Version string
    Deps    map[string]string // module -> url
}

/* -------------------------------------------------------------------------- */
/*  Manifest I/O                                                              */
/* -------------------------------------------------------------------------- */

const manifestFile = "rho.yaml"

func loadManifest() (Manifest, error) {
    data, err := os.ReadFile(manifestFile)
    if err != nil {
        return Manifest{}, err
    }

    v, err := yaml.Load(string(data))
    if err != nil {
        return Manifest{}, err
    }

    m := Manifest{
        Name:    v.Map["name"].Str,
        Version: v.Map["version"].Str,
        Deps:    map[string]string{},
    }

    deps := v.Map["deps"].Map
    for k, dv := range deps {
        m.Deps[k] = dv.Str
    }

    return m, nil
}

func saveManifest(m Manifest) error {
    // minimal YAML writer
    out := "name: " + m.Name + "\n" +
        "version: " + m.Version + "\n" +
        "deps:\n"

    for k, url := range m.Deps {
        out += "  " + k + ": " + url + "\n"
    }

    return os.WriteFile(manifestFile, []byte(out), 0644)
}

/* -------------------------------------------------------------------------- */
/*  Commands                                                                  */
/* -------------------------------------------------------------------------- */

func cmdInit() error {
    if _, err := os.Stat(manifestFile); err == nil {
        return errors.New("rho.yaml already exists")
    }

    m := Manifest{
        Name:    "rho-project",
        Version: "0.1.0",
        Deps:    map[string]string{},
    }

    return saveManifest(m)
}

func cmdAdd(module, url string) error {
    m, err := loadManifest()
    if err != nil {
        return fmt.Errorf("load manifest: %w", err)
    }

    m.Deps[module] = url
    return saveManifest(m)
}

func cmdList() error {
    m, err := loadManifest()
    if err != nil {
        return fmt.Errorf("load manifest: %w", err)
    }

    fmt.Println("name:", m.Name)
    fmt.Println("version:", m.Version)
    fmt.Println("deps:")
    for k, v := range m.Deps {
        fmt.Printf("  %s -> %s\n", k, v)
    }
    return nil
}

/* -------------------------------------------------------------------------- */
/*  CLI Entry Point                                                           */
/* -------------------------------------------------------------------------- */

func main() {
    fs := flag.NewFlagSet("rho pkg", flag.ExitOnError)
    fs.Parse(os.Args[1:])

    if fs.NArg() < 1 {
        fmt.Fprintln(os.Stderr, "usage: rho pkg <init|add|list> [args]")
        os.Exit(2)
    }

    cmd := fs.Arg(0)

    var err error

    switch cmd {
    case "init":
        err = cmdInit()
    case "add":
        if fs.NArg() != 3 {
            fmt.Fprintln(os.Stderr, "usage: rho pkg add <module> <url>")
            os.Exit(2)
        }
        err = cmdAdd(fs.Arg(1), fs.Arg(2))
    case "list":
        err = cmdList()
    default:
        fmt.Fprintln(os.Stderr, "unknown command:", cmd)
        os.Exit(2)
    }

    if err != nil {
        fmt.Fprintln(os.Stderr, "rho pkg:", err)
        os.Exit(1)
    }
}
