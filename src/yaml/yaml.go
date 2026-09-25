//
// @file yaml.go
// @brief Rho YAML Value Model.
//
// Minimal deterministic YAML value representation used by the Rho compiler.
// Supports only the subset needed for Rho config files.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package yaml

/* -------------------------------------------------------------------------- */
/*  YAML Value Model                                                          */
/* -------------------------------------------------------------------------- */

// Value represents a minimal YAML node.
type Value struct {
    Kind  Kind
    Str   string
    Map   map[string]Value
    List  []Value
}

// Kind enumerates YAML node types.
type Kind int

const (
    KindString Kind = iota
    KindMap
    KindList
    // KindNull marks a key that was declared with no value and no
    // child content (for example `features:` at end of file). The
    // loader resolves it to a map or list as soon as content follows.
    KindNull
)
