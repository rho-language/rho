//
// @file emit_go_sqlstore_test.go
// @brief Tests for the generated table-backed store.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package codegen

import (
	"strings"
	"testing"
)

// The store source is emitted verbatim, so a format verb written twice reaches
// the generated project as "%%w" and go vet rejects it. The size of the source
// makes the mistake easy to miss, so it is checked here rather than found when
// a generated registry is tested.
func TestSQLStoreSourceHasSingleFormatVerbs(t *testing.T) {
	if strings.Contains(sqlStoreSource, "%%") {
		t.Error("the generated store source contains a doubled format verb")
	}
}
