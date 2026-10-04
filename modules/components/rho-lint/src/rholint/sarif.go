//
// @file sarif.go
// @brief SARIF 2.1.0, written and read.
//
// One format, for three readers that would otherwise each need a reader of their
// own: an editor's problems panel, GitHub's code scanning, and whatever a
// pipeline does with a file it finds in the build directory. The three of them
// agree on SARIF and on nothing else, so a run writes it — and a run can read it
// too, which is what lets an analyzer that already speaks it be a plugin whose
// whole adapter is a format name.
//
// Two decisions in the writer are about a pipeline rather than about a person,
// and both are the reason this file is not a thin wrapper over a struct:
//
// One run per analyzer, because that is what a SARIF run *is*. A run is one tool
// over one set of results, and a consumer that merges them into a single driver
// loses the attribution — which is the thing that makes "clang-tidy and cppcheck
// disagree about this line" answerable at all.
//
// No timestamps. Every field a run does not need is left out, and a clock reading
// is the one field that guarantees two runs of the same tree differ. A pipeline
// that diffs its own output to decide whether a change introduced a finding
// cannot do that if every run carries a time, so nothing here has one.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rholint

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

// SARIFVersion is the version of the format this component writes and reads.
const SARIFVersion = "2.1.0"

// SARIFSchema is where the format is defined. It is a `$schema` and nothing else:
// no network is reached for it, and a reader that wants to validate the file
// already has the specification.
const SARIFSchema = "https://docs.oasis-open.org/sarif/sarif/v2.1.0/errata01/os/schemas/sarif-schema-2.1.0.json"

// SourceRootBaseID is SARIF's name for the directory a run's paths are relative
// to, which is what lets a report say `src/app.c` instead of the absolute path of
// whichever machine produced it.
const SourceRootBaseID = "%SRCROOT%"

/* -------------------------------------------------------------------------- */
/*  What is written                                                            */
/* -------------------------------------------------------------------------- */

// Rule is one check an analyzer carries: what it is called, what it is for, and
// how bad a match is.
//
// A struct with its own JSON rather than a bare one, because `defaultConfiguration`
// is an object in the format and a bare level string there is a schema violation —
// the kind that makes a strict validator reject the file and therefore every finding
// in it.
//
// Not every analyzer publishes its rules in a form that can be read, so the rules
// in a written run are assembled from two places: what the engine knows about
// them, and what the findings themselves show. An engine that says nothing about a
// rule gets one carrying its name and the worst severity it was seen at, which is
// enough for a consumer to group by and to suppress.
type Rule struct {
	// ID is the fully qualified name a suppression is written against.
	ID string `json:"id"`
	// Name is the analyzer's own short name for it, when it gave one.
	Name string `json:"name,omitempty"`
	// Description is what the rule is for, which is what a consumer shows when
	// somebody asks why a check ran.
	Description string `json:"shortDescription,omitempty"`
	// Severity is what a match is reported at by default.
	//
	// Written as SARIF's own object rather than as a bare level string, because
	// `defaultConfiguration` is a `reportingConfiguration` and a string there is a
	// schema violation — the kind that makes a strict validator reject the whole file
	// and therefore every finding in it.
	Severity Severity `json:"-"`
	// URL is where the analyzer documents it, when it does.
	URL string `json:"helpUri,omitempty"`
}

// Tool is one analyzer, as the driver of a run.
type Tool struct {
	Name string `json:"name"`
	// Version is what the run knew about the analyzer: a version a plugin's spec
	// states, and nothing at all for a program found on PATH whose version nobody
	// asked for. Left out rather than guessed, because a wrong version in a SARIF
	// file is worse than none — it is what code scanning groups alerts by.
	Version string `json:"version,omitempty"`
	// InformationURI is where the analyzer is documented.
	InformationURI string `json:"informationUri,omitempty"`
	// Rules are the checks this analyzer carries, which are the ones its findings
	// mention plus any it described.
	Rules []Rule `json:"rules,omitempty"`
	// Properties carries what SARIF has no field for and this run knows anyway —
	// which engine it was, and whether a plugin provided it. A consumer debugging
	// a run that found nothing needs exactly those two facts, and neither is
	// recoverable from the tool name.
	Properties map[string]string `json:"properties,omitempty"`
}

// The property names this component writes on a tool.
const (
	// PropertySource says where the engine came from: `builtin` or `plugin`.
	PropertySource = "rhoEngineSource"
	// PropertyFormat says how the engine's output is read, which is the answer to
	// "why did this engine's findings look like that".
	PropertyFormat = "rhoEngineFormat"
	// PropertyProgram is the program an engine ran, for an engine that is one.
	PropertyProgram = "rhoEngineProgram"
)

// Invocation is what was run, recorded so a consumer can tell a run that found
// nothing from a run that never ran.
//
// Which is the failure that matters here: an analyzer that refused the
// compilation database exits zero having written nothing, and a report that cannot
// tell that from a clean tree is a green light over an analyzer that is not
// installed.
type Invocation struct {
	// CommandLine is what was run, as one line.
	CommandLine string `json:"commandLine,omitempty"`
	// ExitCode is what it exited with.
	ExitCode int `json:"exitCode"`
	// Successful is whether the engine itself worked, which is not the same as an
	// analyzer exiting non-zero because it found something. Every consumer that
	// gates on a SARIF file has to distinguish those two, and `executionSuccessful`
	// is where the format says to.
	Successful bool `json:"executionSuccessful"`
	// Message is why it did not, when it did not.
	Message string `json:"message,omitempty"`
}

// Run is one analyzer's findings: the tool, how it was invoked, and what it said.
//
// Together because they answer three questions a report has to answer together —
// what is this, what ran, and did it work — and separating them means a consumer
// showing findings has to correlate three lists to learn whether any of them came
// from a tool that was not there.
type Run struct {
	Tool       Tool         `json:"tool"`
	Invocation Invocation   `json:"invocations"`
	Results    []Diagnostic `json:"results"`
}

// logFile is a SARIF file as written: one run per analyzer.
type logFile struct {
	Schema  string    `json:"$schema"`
	Version string    `json:"version"`
	Runs    []fileRun `json:"runs"`
}

// fileRun is one run as SARIF states it, which is not what a Run holds: the
// results are a different shape, and the base ids live on the run.
type fileRun struct {
	Tool fileTool `json:"tool"`
	// Invocations is an *array* even though a run starts one thing.
	//
	// The format says `invocations` is `invocation[]`, and `executionSuccessful` on
	// it is required — so an object there is a schema violation, and a validator that
	// rejects the run rejects every finding in the file with it. A list of one is
	// what the format means by "how this run went".
	Invocations       []Invocation       `json:"invocations"`
	Results           []fileResult       `json:"results"`
	OriginalURIBaseID map[string]uriBase `json:"originalUriBaseIds,omitempty"`
	// WorkingDirectory is how a human reading the file knows what the relative
	// paths are relative to, which `originalUriBaseIds` states as a URI and
	// nothing says as a directory.
	WorkingDirectory string `json:"workingDirectory,omitempty"`
}

// fileTool is the driver of a run as SARIF states it.
type fileTool struct {
	Name           string            `json:"name"`
	Version        string            `json:"version,omitempty"`
	InformationURI string            `json:"informationUri,omitempty"`
	Rules          []ruleFile        `json:"rules,omitempty"`
	Properties     map[string]string `json:"properties,omitempty"`
}

type uriBase struct {
	URI string `json:"uri"`
}

// ruleFile is a rule as SARIF states it, which nests the severity one level down
// under `defaultConfiguration` rather than beside the id.
type ruleFile struct {
	ID          string      `json:"id"`
	Name        string      `json:"name,omitempty"`
	Description string      `json:"shortDescription,omitempty"`
	Default     *ruleConfig `json:"defaultConfiguration,omitempty"`
	Help        string      `json:"helpUri,omitempty"`
}

type ruleConfig struct {
	Level string `json:"level"`
}

// fileResult is one finding as SARIF states it.
type fileResult struct {
	RuleID    string       `json:"ruleId"`
	Level     string       `json:"level"`
	Message   fileMessage  `json:"message"`
	Locations []fileRegion `json:"locations,omitempty"`
}

type fileMessage struct {
	Text string `json:"text"`
}

type fileRegion struct {
	PhysicalLocation physicalLocation `json:"physicalLocation"`
}

type physicalLocation struct {
	ArtifactLocation artifactLocation `json:"artifactLocation"`
	Region           region           `json:"region"`
}

type artifactLocation struct {
	URI string `json:"uri"`
	// URIBaseID says the URI is relative to one of the run's base ids rather than
	// to wherever the reader happens to be standing.
	URIBaseID string `json:"uriBaseId,omitempty"`
}

type region struct {
	StartLine   int `json:"startLine"`
	StartColumn int `json:"startColumn,omitempty"`
	EndLine     int `json:"endLine,omitempty"`
	EndColumn   int `json:"endColumn,omitempty"`
}

// WriteSARIF writes a run's findings as a SARIF file.
//
// One run per analyzer, in the order the engines were chosen, so two runs of the
// same tree produce the same file. `root` is the directory the paths are made
// relative to: given, every path inside it is written relative to it and the run
// carries the base id; omitted, paths are written as the analyzers gave them and
// the run carries no base, because a `%SRCROOT%` pointing at nothing sends a
// consumer looking for files that are not there.
func WriteSARIF(w io.Writer, runs []Run, root string) error {
	base := sourceRoot(root)
	out := logFile{
		Schema:  SARIFSchema,
		Version: SARIFVersion,
		Runs:    make([]fileRun, 0, len(runs)),
	}

	for _, run := range runs {
		file := fileRun{
			Tool:        fileToolFor(run.Tool),
			Invocations: []Invocation{run.Invocation},
			Results:     make([]fileResult, 0, len(run.Results)),
		}
		if base != "" {
			file.OriginalURIBaseID = map[string]uriBase{SourceRootBaseID: {URI: base}}
			file.WorkingDirectory = workingDirectory(root)
		}
		for _, one := range run.Results {
			file.Results = append(file.Results, sarifResult(one, root))
		}
		out.Runs = append(out.Runs, file)
	}

	encoded, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("the findings could not be encoded as SARIF: %w", err)
	}
	if _, err := w.Write(append(encoded, '\n')); err != nil {
		return fmt.Errorf("the findings could not be written: %w", err)
	}
	return nil
}

// fileToolFor is a tool as SARIF states it.
func fileToolFor(tool Tool) fileTool {
	out := fileTool{
		Name:           tool.Name,
		Version:        tool.Version,
		InformationURI: tool.InformationURI,
		Properties:     tool.Properties,
	}
	for _, rule := range tool.Rules {
		written := ruleFile{
			ID:          rule.ID,
			Name:        rule.Name,
			Description: rule.Description,
			Help:        rule.URL,
		}
		if rule.Severity != "" {
			written.Default = &ruleConfig{Level: string(sarifLevel(rule.Severity))}
		}
		out.Rules = append(out.Rules, written)
	}
	return out
}

// sarifLevel is a severity as SARIF spells it.
//
// The names coincide with the ones here, and that coincidence is the reason the
// severities are named this way rather than after any analyzer's: the three an
// analyzer means and the three SARIF means are the same three, so the translation is
// the identity and there is no table to be wrong about.
func sarifLevel(severity Severity) Severity {
	switch severity {
	case SeverityError:
		return SeverityError
	case SeverityWarning:
		return SeverityWarning
	case SeverityNote:
		return SeverityNote
	case "":
		return SeverityNone
	default:
		// A severity this component has not met becomes `none` rather than being
		// dropped: a finding recorded as present and not worth reporting is still a
		// finding, and a consumer can see it and count it.
		return SeverityNone
	}
}

// sarifResult is one finding as SARIF states it.
func sarifResult(one Diagnostic, root string) fileResult {
	out := fileResult{
		RuleID:  one.Rule,
		Level:   string(sarifLevel(one.Severity)),
		Message: fileMessage{Text: one.Message},
	}
	if one.File != "" {
		location := fileRegion{PhysicalLocation: physicalLocation{
			ArtifactLocation: artifactLocation{URI: one.File},
			Region: region{
				StartLine:   one.Line,
				StartColumn: one.Column,
				EndLine:     one.EndLine,
				EndColumn:   one.EndColumn,
			},
		}}
		if sourceRoot(root) != "" {
			if relative, ok := relativeTo(root, one.File); ok {
				location.PhysicalLocation.ArtifactLocation = artifactLocation{
					URI:       relative,
					URIBaseID: SourceRootBaseID,
				}
			}
		}
		out.Locations = append(out.Locations, location)
	}
	return out
}

// relativeTo is a path relative to a root, and whether it is one at all.
//
// A file outside the root has no relative path to it, and writing `../vendor/x.c`
// with a base id that points at the root is a path a consumer resolves to
// something that is not the file. Those are written as they are instead.
func relativeTo(root string, path string) (string, bool) {
	if root == "" || path == "" {
		return "", false
	}
	// Already relative — which is the shape a Diagnostic holds, so this is the common
	// case rather than the exceptional one. It is relative to the root by
	// construction, so it is passed through and gets the base id like any other.
	if !filepath.IsAbs(path) {
		return filepath.ToSlash(path), true
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == "" {
		return "", false
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}
	return relative, true
}

// sourceRoot is a directory as the URI a SARIF base id carries.
//
// The trailing slash is what makes it a prefix: without it, `%SRCROOT%/src/app.c`
// is nonsense to a consumer that concatenates.
func sourceRoot(root string) string {
	absolute := workingDirectory(root)
	if absolute == "" {
		return ""
	}
	// A Windows path is not a URI and `C:/x` reads as a scheme. Three slashes and
	// the escaping are what a `file:` URI is, which every consumer understands
	// whatever it runs on.
	if !strings.HasPrefix(absolute, "/") {
		absolute = "/" + absolute
	}
	return "file://" + filepath.ToSlash(absolute) + "/"
}

// workingDirectory is a root as an absolute path with no trailing separator.
func workingDirectory(root string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return ""
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return filepath.Clean(root)
	}
	return filepath.Clean(absolute)
}

/* -------------------------------------------------------------------------- */
/*  Reading SARIF back                                                         */
/* -------------------------------------------------------------------------- */

// ReadSARIF reads findings out of a SARIF file.
//
// The other direction, and needed for the same reason the writing is: an analyzer
// that already speaks SARIF — which is most of them now — should be a plugin
// whose adapter is a format name rather than a parser somebody maintains here.
//
// Every run in the file becomes findings attributed to that run's tool, because a
// file with three runs is three analyzers' work and the attribution is the only
// thing that says which is which.
func ReadSARIF(data []byte) ([]Diagnostic, error) {
	document := logFileIn{}
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("this is not a SARIF file: %w", err)
	}
	if strings.TrimSpace(document.Version) == "" {
		return nil, fmt.Errorf("this is a JSON file with no SARIF version in it, so it is not one")
	}

	found := []Diagnostic{}
	for _, run := range document.Runs {
		engine := run.Tool.Name
		if strings.TrimSpace(engine) == "" {
			engine = "unknown"
		}
		for _, result := range run.Results {
			found = append(found, fromSarifResult(result, engine, run.OriginalURIBaseID))
		}
	}
	return found, nil
}

// logFileIn is a SARIF file as read, which is deliberately a separate shape from
// what is written: a SARIF file is written by other tools too, and the reader has
// to accept their spelling rather than this component's.
type logFileIn struct {
	Version string      `json:"version"`
	Runs    []fileRunIn `json:"runs"`
}

type fileRunIn struct {
	Tool              Tool               `json:"tool"`
	Results           []fileResultIn     `json:"results"`
	OriginalURIBaseID map[string]uriBase `json:"originalUriBaseIds"`
}

type fileResultIn struct {
	RuleID  string      `json:"ruleId"`
	Level   string      `json:"level"`
	Message fileMessage `json:"message"`
	// Locations is read rather than a single location, because a result may name
	// several and the first is the one that is where it is.
	Locations []fileRegionIn `json:"locations"`
}

type fileRegionIn struct {
	PhysicalLocation struct {
		ArtifactLocation struct {
			URI       string `json:"uri"`
			URIBaseID string `json:"uriBaseId"`
		} `json:"artifactLocation"`
		Region struct {
			StartLine   int `json:"startLine"`
			StartColumn int `json:"startColumn"`
			EndLine     int `json:"endLine"`
			EndColumn   int `json:"endColumn"`
		} `json:"region"`
	} `json:"physicalLocation"`
}

// fromSarifResult is one result as a finding of this component's shape.
func fromSarifResult(result fileResultIn, engine string, bases map[string]uriBase) Diagnostic {
	out := Diagnostic{
		Rule:     result.RuleID,
		Severity: sarifLevel(Severity(result.Level)),
		Message:  result.Message.Text,
		Engine:   engine,
	}
	if len(result.Locations) == 0 {
		return out
	}
	place := result.Locations[0].PhysicalLocation
	out.File = resolveURI(place.ArtifactLocation.URI, place.ArtifactLocation.URIBaseID, bases)
	out.Line = place.Region.StartLine
	out.Column = place.Region.StartColumn
	out.EndLine = place.Region.EndLine
	out.EndColumn = place.Region.EndColumn
	return out
}

// resolveURI is a result's `uri` as a path.
//
// A base id is resolved when the file declares one, because that is the case where
// the path on its own means nothing — `%SRCROOT%/src/app.c` is a path only
// relative to something. A base that cannot be resolved leaves the URI as it is,
// which is a path a reader may still be able to use, rather than a path with a
// token in it.
func resolveURI(uri string, baseID string, bases map[string]uriBase) string {
	if strings.TrimSpace(uri) == "" {
		return ""
	}
	if baseID == "" {
		return filepath.FromSlash(uri)
	}
	base, known := bases[baseID]
	if !known {
		return filepath.FromSlash(uri)
	}
	directory := base.URI
	if !strings.HasPrefix(directory, "file://") {
		return filepath.FromSlash(uri)
	}
	directory = filepath.FromSlash(strings.TrimPrefix(directory, "file://"))
	// The base is a directory with a trailing slash, which `FromSlash` keeps and
	// `Join` does not need.
	return filepath.Join(filepath.Clean(directory), filepath.FromSlash(uri))
}
