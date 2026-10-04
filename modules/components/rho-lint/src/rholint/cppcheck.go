//
// @file cppcheck.go
// @brief cppcheck's XML report, as the shapes it is written in.
//
// Kept in its own file because it is the one output format here that is somebody
// else's schema rather than a line of text. `gcc` and SARIF are the format's name
// and cppcheck's is a document with a version attribute and a vocabulary of its
// own, and the three of them earn different amounts of care for that reason.
//
// The types are named after the elements they read. There is no translation layer
// to this component's own shape here: `ReadCPPCheck` in the engine file does that,
// and the reason it is there rather than here is that the normalization — severity
// mapping, rule qualification, making paths relative — is the same for every format
// and there must be exactly one of it.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rholint

// cppcheckReport is a `<results>` document.
type cppcheckReport struct {
	XMLName struct{} `xml:"results"`
	// Version is the report's own schema version. Read and not used, and said so here
	// rather than left as an unused field: it is the thing to quote in a bug report
	// about a finding that should not be there.
	Version string `xml:"version,attr"`
	// Cppcheck is the analyzer's own version, which is the other thing to quote.
	Cppcheck struct {
		Version string `xml:"version,attr"`
	} `xml:"cppcheck"`
	// Errors is the `<errors>` wrapper, spelled out rather than flattened.
	//
	// A field tagged `xml:"error"` matches a *direct* child only, and every `<error>`
	// in a real report sits inside `<errors>`. Reading a report and getting zero
	// findings out of it is the failure mode this file exists to prevent, so the
	// nesting is written down rather than assumed.
	Errors struct {
		Error []cppcheckError `xml:"error"`
	} `xml:"errors"`
}

// findings are the errors in a report, in document order.
func (r cppcheckReport) findings() []cppcheckError { return r.Errors.Error }

// cppcheckError is one `<error>`, which is one finding.
type cppcheckError struct {
	// ID is the check's name, in cppcheck's own spelling: `unusedVariable`,
	// `nullPointer`, `missingReturn`.
	ID string `xml:"id,attr"`
	// Severity is one of `error`, `warning`, `style`, `performance`,
	// `portability`, `information`, `debug`, `internal`.
	Severity string `xml:"severity,attr"`
	// Verbose is a longer sentence than `Message`, which is the one-line form. The
	// long one is not read: `Message` is what a report shows, and a run that
	// printed both would print the same finding twice in two lengths.
	Message string `xml:"msg,attr"`
	// CWE is the Common Weakness Enumeration number, when cppcheck knows one.
	// Not carried into a finding: this component's shape has nowhere to put it, and
	// adding a field for one analyzer's extra metadata is how a shape stops being a
	// shape.
	CWE string `xml:"cwe,attr"`
	// Inconclusive says cppcheck was not sure. Not carried either, and this is a
	// deliberate loss rather than an oversight: an inconclusive finding is one a
	// person should look at and a run should say nothing about, and the way to say it
	// in this shape is the message, which is why `inconclusive:` is prefixed to the
	// one that is kept.
	Inconclusive string `xml:"inconclusive,attr"`
	// Locations are the places the finding applies to, most specific first.
	Locations []cppcheckLocation `xml:"location"`
	// Calls is where the finding came from, for the checks that follow a call.
	Calls []cppcheckLocation `xml:"call"`
}

// cppcheckLocation is one `<location>` or `<call>`.
type cppcheckLocation struct {
	File   string `xml:"file,attr"`
	Line   string `xml:"line,attr"`
	Column string `xml:"column,attr"`
	Info   string `xml:"info,attr"`
}

// Number is the line, which cppcheck writes as an attribute and therefore as a
// string.
//
// A string because that is what the attribute is, and parsing it here rather than
// with a custom unmarshaller keeps the one place that has to cope with cppcheck
// writing an empty attribute — which it does, for a finding with no line — being
// this function rather than a decoder somewhere under it.
func (l cppcheckLocation) Number() int { return atoi(l.Line) }

// ColumnNumber is the column, which cppcheck also writes as a string and omits for
// every finding it does not have one for.
func (l cppcheckLocation) ColumnNumber() int { return atoi(l.Column) }
