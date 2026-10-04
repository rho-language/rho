//
// @file semver.go
// @brief Rho Semantic Versioning - Enterprise v3.
//
// Implements semantic version parsing and comparison for Rho modules.
// Supports:
//   - version parsing (major.minor.patch)
//   - version comparison
//   - constraint evaluation (>=1.0.0 <2.0.0)
//   - validation
//
// Used by:
//   - resolver.go
//   - installer.go
//   - update.go
//   - validator.go
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package core

import (
    "fmt"
    "strconv"
    "strings"
)

/* -------------------------------------------------------------------------- */
/*  Types                                                                      */
/* -------------------------------------------------------------------------- */

type Semver struct {
    Major int
    Minor int
    Patch int
}

/* -------------------------------------------------------------------------- */
/*  Parse Version                                                               */
/* -------------------------------------------------------------------------- */

func ParseSemver(s string) (Semver, error) {
    parts := strings.Split(s, ".")
    if len(parts) != 3 {
        return Semver{}, fmt.Errorf("invalid semver: %s", s)
    }

    major, err := strconv.Atoi(parts[0])
    if err != nil {
        return Semver{}, fmt.Errorf("invalid major version: %s", s)
    }

    minor, err := strconv.Atoi(parts[1])
    if err != nil {
        return Semver{}, fmt.Errorf("invalid minor version: %s", s)
    }

    patch, err := strconv.Atoi(parts[2])
    if err != nil {
        return Semver{}, fmt.Errorf("invalid patch version: %s", s)
    }

    return Semver{major, minor, patch}, nil
}

/* -------------------------------------------------------------------------- */
/*  Validate Semver                                                             */
/* -------------------------------------------------------------------------- */

func IsValidSemver(s string) bool {
    _, err := ParseSemver(s)
    return err == nil
}

/* -------------------------------------------------------------------------- */
/*  Compare Versions                                                            */
/* -------------------------------------------------------------------------- */

func CompareSemver(a, b Semver) int {
    if a.Major != b.Major {
        return compareInt(a.Major, b.Major)
    }
    if a.Minor != b.Minor {
        return compareInt(a.Minor, b.Minor)
    }
    return compareInt(a.Patch, b.Patch)
}

func compareInt(a, b int) int {
    if a < b {
        return -1
    }
    if a > b {
        return 1
    }
    return 0
}

/* -------------------------------------------------------------------------- */
/*  Constraint Evaluation                                                       */
/* -------------------------------------------------------------------------- */

func SemverSatisfies(version string, constraint string) (bool, error) {
    v, err := ParseSemver(version)
    if err != nil {
        return false, err
    }

    // Example constraint: ">=1.0.0 <2.0.0"
    parts := strings.Fields(constraint)

    for _, part := range parts {
        if strings.HasPrefix(part, ">=") {
            c := strings.TrimPrefix(part, ">=")
            cv, err := ParseSemver(c)
            if err != nil {
                return false, err
            }
            if CompareSemver(v, cv) < 0 {
                return false, nil
            }
        } else if strings.HasPrefix(part, "<=") {
            c := strings.TrimPrefix(part, "<=")
            cv, err := ParseSemver(c)
            if err != nil {
                return false, err
            }
            if CompareSemver(v, cv) > 0 {
                return false, nil
            }
        } else if strings.HasPrefix(part, ">") {
            c := strings.TrimPrefix(part, ">")
            cv, err := ParseSemver(c)
            if err != nil {
                return false, err
            }
            if CompareSemver(v, cv) <= 0 {
                return false, nil
            }
        } else if strings.HasPrefix(part, "<") {
            c := strings.TrimPrefix(part, "<")
            cv, err := ParseSemver(c)
            if err != nil {
                return false, err
            }
            if CompareSemver(v, cv) >= 0 {
                return false, nil
            }
        } else {
            // Exact match
            cv, err := ParseSemver(part)
            if err != nil {
                return false, err
            }
            if CompareSemver(v, cv) != 0 {
                return false, nil
            }
        }
    }

    return true, nil
}
